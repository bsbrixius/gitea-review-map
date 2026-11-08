import {addDelegatedEventListener, hideElem, showElem} from '../utils/dom.ts';
import {POST} from '../modules/fetch.ts';

// nested types marshal with their Go field names, see services/reviewmap/pagedata.go
export type ReviewMapRef = {
  File: string,
  NameHash: string,
  StartLine: number,
  EndLine: number,
  Valid: boolean,
};

export type ReviewMapBullet = {
  Index: number,
  Hash: string,
  Text: string,
  Orphan: boolean,
  Refs: ReviewMapRef[],
  AddedLines: number,
  RemovedLines: number,
};

export type ReviewMapPageData = {
  ignoredReason: string,
  issueLink: string,
  toggleUrl: string,
  canReview: boolean,
  myReviewed: string[],
  myApprovedRefs: string[] | null,
  bulletApproval: {approved: number, total: number}[] | null,
  bullets: ReviewMapBullet[],
  commentLinks: Record<string, {id: number, url: string}[]>,
  coverage: {TotalChanged: number, MappedChanged: number, UnmappedFiles: string[]},
  ignoreGlobs: string[],
  invalidRefCount: number,
  orphanCount: number,
  progress: {userId: number, name: string, done: number, total: number}[],
};

const filterStorageKey = 'review-map-filter';

export function isFilesPage(): boolean {
  return document.querySelector('#diff-file-boxes') !== null;
}

function boxFileName(box: Element): string {
  return box.getAttribute('data-new-filename') || box.getAttribute('data-old-filename') || '';
}

// clearBulletFilter reverts everything the filter changed: it only touches the diff file boxes'
// "tw-hidden" class and row-level "tw-hidden"/"review-map-line-highlight" classes inside them.
// No other feature hides whole rows inside diff file boxes (diff folding uses "data-folded" on the box),
// so un-hiding every row in every box is safe and keeps the function free of hidden state.
export function clearBulletFilter(root: Document | HTMLElement): void {
  for (const box of root.querySelectorAll('#diff-file-boxes .diff-file-box')) {
    box.classList.remove('tw-hidden');
    for (const row of box.querySelectorAll('tr')) {
      row.classList.remove('tw-hidden', 'review-map-line-highlight');
    }
  }
}

function filterBoxRows(box: Element, refs: ReviewMapRef[]): void {
  let lastRight: number | null = null; // last non-empty new-side line number seen so far
  let lastCodeRowVisible = false;
  let pendingTagRows: Element[] = []; // hunk headers waiting for a visible code row after them
  for (const row of box.querySelectorAll('table.chroma[data-path] tr')) {
    if (row.classList.contains('add-comment')) {
      // a comment thread row belongs to the code row right above it
      if (!lastCodeRowVisible) hideElem(row);
      continue;
    }
    if (row.classList.contains('tag-code')) {
      // a hunk header is kept only when a code row until the next hunk header is visible
      for (const tagRow of pendingTagRows) hideElem(tagRow);
      pendingTagRows = [row];
      continue;
    }
    const lineNumAttr = row.querySelector('td.lines-num-new')?.getAttribute('data-line-num');
    let lineNum: number | null;
    if (lineNumAttr) {
      lineNum = parseInt(lineNumAttr, 10);
      lastRight = lineNum;
    } else {
      lineNum = lastRight; // deletion rows without a new-side number anchor on the previous row
    }
    const visible = lineNum !== null && refs.some((ref) => lineNum >= ref.StartLine && lineNum <= ref.EndLine);
    if (visible) {
      row.classList.add('review-map-line-highlight');
      pendingTagRows = []; // their following code row is visible, so the pending headers stay visible
    } else {
      hideElem(row);
    }
    lastCodeRowVisible = visible;
  }
  for (const tagRow of pendingTagRows) hideElem(tagRow);
}

// applyBulletFilter narrows the diff to the lines implementing one review-map bullet.
// It returns false when no file box matches any ref of the bullet: the diff is left untouched
// so the caller can show the fallback and scroll to the first ref's file anchor instead.
export function applyBulletFilter(bullet: {Refs: ReviewMapRef[]}, root: Document | HTMLElement): boolean {
  const boxes = Array.from(root.querySelectorAll<HTMLElement>('#diff-file-boxes .diff-file-box'));
  const refsByBox = boxes.map((box) => ({box, refs: bullet.Refs.filter((ref) => ref.File === boxFileName(box))}));
  clearBulletFilter(root); // also makes applying another bullet retarget in place
  if (refsByBox.every((entry) => !entry.refs.length)) return false;
  for (const {box, refs} of refsByBox) {
    if (!refs.length) {
      hideElem(box);
      continue;
    }
    filterBoxRows(box, refs);
  }
  return true;
}

// setActiveBullet marks the sidebar card of the applied bullet; pass null to clear.
export function setActiveBullet(panel: HTMLElement, index: number | null): void {
  for (const bullet of panel.querySelectorAll('.review-map-bullet')) {
    bullet.classList.toggle('review-map-active', bullet.getAttribute('data-bullet-index') === String(index));
  }
}

// nextUnapprovedBulletIndex returns the first bullet (by panel order) whose valid refs are not
// all approved yet, skipping the given index; -1 when every bullet is done.
export function nextUnapprovedBulletIndex(approval: {approved: number, total: number}[], skipIndex: number): number {
  for (const [i, a] of approval.entries()) {
    if (i !== skipIndex && a.total > 0 && a.approved < a.total) return i;
  }
  return -1;
}

export function initReviewMapPanel() {
  const panel = document.querySelector<HTMLElement>('#review-map-panel');
  if (!panel) return;
  const data = window.config.pageData.reviewMap;
  if (!data) return;

  addDelegatedEventListener<HTMLInputElement, Event>(panel, 'change', '.review-map-check', (checkbox) => {
    const bullet = checkbox.closest('.review-map-bullet') as HTMLElement;
    // fire-and-forget: the checkbox keeps its clicked state, progress counts refresh on the next page load
    POST(data.toggleUrl, {data: {hash: bullet.getAttribute('data-bullet-hash'), reviewed: checkbox.checked}});
  });

  if (!isFilesPage()) {
    // conversation page: remember the clicked bullet and let the files page apply the filter
    addDelegatedEventListener<HTMLElement, MouseEvent>(panel, 'click', '.review-map-bullet', (bullet, e) => {
      if ((e.target as HTMLElement).closest('.review-map-check, .review-map-comment')) return;
      e.preventDefault();
      sessionStorage.setItem(filterStorageKey, bullet.getAttribute('data-bullet-index')!);
      window.location.assign(`${data.issueLink}/files`);
    });
    return;
  }

  const showAllButton = panel.querySelector<HTMLElement>('#review-map-show-all')!;
  const fallbackBox = panel.querySelector<HTMLElement>('#review-map-fallback')!;

  // while a bullet filter is active, the diff header shows that bullet's numbers instead of the PR totals
  const statsBox = document.querySelector<HTMLElement>('.diff-detail-stats');
  const statsTpl = panel.querySelector('#review-map-filtered-stats-tpl')?.getAttribute('data-tpl') ?? null;
  let originalStatsHTML: string | null = null;
  const showBulletStats = (bullet: ReviewMapBullet, visibleCount: number, totalCount: number) => {
    if (!statsBox || !statsTpl) return;
    if (originalStatsHTML === null) originalStatsHTML = statsBox.innerHTML;
    const label = statsTpl.replace('{n}', String(visibleCount)).replace('{total}', String(totalCount));
    statsBox.innerHTML = `${label} <span class="tw-text-green">+${bullet.AddedLines}</span> <span class="tw-text-red">−${bullet.RemovedLines}</span>`;
  };
  const restoreStats = () => {
    if (originalStatsHTML !== null && statsBox) {
      statsBox.innerHTML = originalStatsHTML;
      originalStatsHTML = null;
    }
  };

  const totalBoxCount = document.querySelectorAll('#diff-file-boxes .diff-file-box').length;
  const applyBulletByIndex = (index: number) => {
    hideElem(fallbackBox);
    const bullet = data.bullets.find((it) => it.Index === index);
    if (!bullet) return;
    if (applyBulletFilter(bullet, document)) {
      showElem(showAllButton);
      setActiveBullet(panel, index);
      sessionStorage.setItem(filterStorageKey, String(index)); // reloads keep the active filter
      const visibleCount = document.querySelectorAll('#diff-file-boxes .diff-file-box:not(.tw-hidden)').length;
      showBulletStats(bullet, visibleCount, totalBoxCount);
      document.querySelector('tr.review-map-line-highlight')?.scrollIntoView({block: 'center'});
    } else {
      setActiveBullet(panel, null);
      restoreStats();
      showElem(fallbackBox);
      const nameHash = bullet.Refs[0]?.NameHash;
      if (nameHash) document.querySelector(`#diff-${nameHash}`)?.scrollIntoView();
    }
  };

  addDelegatedEventListener<HTMLElement, MouseEvent>(panel, 'click', '.review-map-bullet', (bullet, e) => {
    if ((e.target as HTMLElement).closest('.review-map-check, .review-map-ref-check, .review-map-approve-chip, .review-map-comment')) return;
    e.preventDefault();
    applyBulletByIndex(parseInt(bullet.getAttribute('data-bullet-index')!, 10));
  });

  // per-file approval inside a bullet: recount the card's chip from its checkboxes
  const refreshChip = (bullet: HTMLElement) => {
    const chip = bullet.querySelector<HTMLElement>('.review-map-approve-chip');
    if (!chip) return;
    const checks = Array.from(bullet.querySelectorAll<HTMLInputElement>('.review-map-ref-check'));
    const approved = checks.filter((c) => c.checked).length;
    chip.setAttribute('data-approved', String(approved));
    chip.setAttribute('data-total', String(checks.length));
    chip.querySelector('.review-map-chip-count')!.textContent = `${approved}/${checks.length}`;
    chip.classList.toggle('review-map-done', checks.length > 0 && approved === checks.length);
    chip.classList.toggle('tw-bg-green', checks.length > 0 && approved === checks.length);
    chip.classList.toggle('tw-text-white', checks.length > 0 && approved === checks.length);
  };

  const panelApproval = (): {approved: number, total: number}[] =>
    Array.from(panel.querySelectorAll<HTMLElement>('.review-map-bullet'), (bullet) => {
      const checks = bullet.querySelectorAll('.review-map-ref-check');
      return {approved: bullet.querySelectorAll('.review-map-ref-check:checked').length, total: checks.length};
    });

  const maybeAdvance = (bullet: HTMLElement) => {
    const index = parseInt(bullet.getAttribute('data-bullet-index')!, 10);
    const next = nextUnapprovedBulletIndex(panelApproval(), index);
    if (next >= 0) window.setTimeout(() => applyBulletByIndex(next), 600); // bullet complete: walk to the next one
  };

  addDelegatedEventListener<HTMLInputElement, Event>(panel, 'change', '.review-map-ref-check', (checkbox) => {
    const bullet = checkbox.closest('.review-map-bullet') as HTMLElement;
    const hash = bullet.getAttribute('data-bullet-hash')!;
    const refIndex = parseInt(checkbox.getAttribute('data-ref-index')!, 10);
    POST(data.toggleUrl, {data: {hash, refIndex, reviewed: checkbox.checked}});
    refreshChip(bullet);
    maybeAdvance(bullet);
  });

  addDelegatedEventListener<HTMLButtonElement, MouseEvent>(panel, 'click', '.review-map-approve-chip', (chip, e) => {
    e.preventDefault();
    const bullet = chip.closest('.review-map-bullet') as HTMLElement;
    const hash = bullet.getAttribute('data-bullet-hash')!;
    const checks = Array.from(bullet.querySelectorAll<HTMLInputElement>('.review-map-ref-check'));
    const target = chip.getAttribute('data-approved') !== chip.getAttribute('data-total'); // approve all unless everything is approved
    for (const check of checks) {
      if (check.checked === target) continue;
      check.checked = target;
      POST(data.toggleUrl, {data: {hash, refIndex: parseInt(check.getAttribute('data-ref-index')!, 10), reviewed: target}});
    }
    refreshChip(bullet);
    if (target) maybeAdvance(bullet);
  });

  showAllButton.addEventListener('click', () => {
    clearBulletFilter(document);
    hideElem(showAllButton);
    setActiveBullet(panel, null);
    restoreStats();
    sessionStorage.removeItem(filterStorageKey);
  });

  const storedIndex = sessionStorage.getItem(filterStorageKey);
  if (storedIndex !== null) {
    applyBulletByIndex(parseInt(storedIndex, 10)); // kept until "Show all files": reloads stay filtered
  }
}
