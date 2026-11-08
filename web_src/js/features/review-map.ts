import {addDelegatedEventListener, hideElem, showElem} from '../utils/dom.ts';
import {POST} from '../modules/fetch.ts';
import {localUserSettings} from '../modules/user-settings.ts';

// nested types marshal with their Go field names, see services/reviewmap/pagedata.go
export type ReviewMapRef = {
  File: string,
  NameHash: string,
  StartLine: number,
  EndLine: number,
  Valid: boolean,
  Note: string,
};

export type ReviewMapBullet = {
  Index: number,
  Hash: string,
  Text: string,
  Orphan: boolean,
  Refs: ReviewMapRef[],
  AddedLines: number,
  RemovedLines: number,
  Summary: string,
  Details: string[] | null,
};

export type ReviewMapPageData = {
  ignoredReason: string,
  issueLink: string,
  toggleUrl: string,
  canReview: boolean,
  myReviewed: string[],
  myApprovedRefs: string[] | null,
  bulletApproval: {approved: number, total: number}[] | null,
  myFlagged: string[] | null,
  bulletFlags: number[] | null,
  flaggedCount: number,
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
// "tw-hidden" class and row-level "tw-hidden"/"review-map-line-highlight"/"review-map-strip-open"
// classes inside them. No other feature hides whole rows inside diff file boxes (diff folding uses
// "data-folded" on the box), so un-hiding every row in every box is safe and keeps the function
// free of hidden state.
export function clearBulletFilter(root: Document | HTMLElement): void {
  for (const box of root.querySelectorAll('#diff-file-boxes .diff-file-box')) {
    box.classList.remove('tw-hidden');
    for (const row of box.querySelectorAll('tr')) {
      row.classList.remove('tw-hidden', 'review-map-line-highlight', 'review-map-strip-open');
    }
  }
}

function filterBoxRows(box: Element, refs: ReviewMapRef[], activeIndex: number): void {
  let lastRight: number | null = null; // last non-empty new-side line number seen so far
  let lastCodeRowVisible = false;
  let pendingTagRows: Element[] = []; // hunk headers waiting for a visible code row after them
  for (const row of box.querySelectorAll('table.chroma[data-path] tr')) {
    if (row.classList.contains('review-map-strip')) {
      // a summary strip stays only for the change being applied
      if (row.getAttribute('data-bullet-index') === String(activeIndex)) row.classList.remove('tw-hidden');
      else hideElem(row);
      continue;
    }
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
export function applyBulletFilter(bullet: {Index: number, Refs: ReviewMapRef[]}, root: Document | HTMLElement): boolean {
  const boxes = Array.from(root.querySelectorAll<HTMLElement>('#diff-file-boxes .diff-file-box'));
  const refsByBox = boxes.map((box) => ({box, refs: bullet.Refs.filter((ref) => ref.File === boxFileName(box))}));
  clearBulletFilter(root); // also makes applying another bullet retarget in place
  if (refsByBox.every((entry) => !entry.refs.length)) return false;
  for (const {box, refs} of refsByBox) {
    if (!refs.length) {
      hideElem(box);
      continue;
    }
    filterBoxRows(box, refs, bullet.Index);
  }
  return true;
}

// --- plain-english strips ------------------------------------------------------

// buildStripRows inserts one collapsed strip row above the first mapped line of every
// (bullet x file) pair that has a row to anchor on. Strips always exist (they anchor the
// change in the file) and expand for the selected change; body = the file note, else the
// change summary.
export function buildStripRows(data: ReviewMapPageData, root: Document | HTMLElement): void {
  for (const box of root.querySelectorAll<HTMLElement>('#diff-file-boxes .diff-file-box')) {
    for (const bullet of data.bullets) {
      const refs = bullet.Refs.filter((ref) => ref.File === boxFileName(box));
      if (!refs.length) continue;
      const anchor = firstMappedRow(box, refs);
      if (!anchor) continue;
      anchor.before(makeStripRow(bullet, refs.find((ref) => ref.Note) || null));
    }
  }
}

// firstMappedRow walks the box rows tracking the new-side line number (deletion rows anchor
// on the previous row's number) and returns the first row inside any ref range.
function firstMappedRow(box: Element, refs: ReviewMapRef[]): Element | null {
  let lastRight: number | null = null;
  for (const row of box.querySelectorAll('table.chroma[data-path] tr')) {
    if (row.classList.contains('review-map-strip') || row.classList.contains('add-comment')) continue;
    const lineNumAttr = row.querySelector('td.lines-num-new')?.getAttribute('data-line-num');
    let lineNum: number | null = null; // deletion rows without a new-side number anchor on the previous row
    if (lineNumAttr) {
      lineNum = parseInt(lineNumAttr, 10);
      lastRight = lineNum;
    } else {
      lineNum = lastRight;
    }
    if (lineNum !== null && refs.some((ref) => lineNum >= ref.StartLine && lineNum <= ref.EndLine)) return row;
  }
  return null;
}

function makeStripRow(bullet: ReviewMapBullet, noteRef: ReviewMapRef | null): HTMLElement {
  const tpl = document.querySelector('#review-map-strip-tpl');
  const moreLabel = tpl?.getAttribute('data-more') || 'More';
  const lessLabel = tpl?.getAttribute('data-less') || 'Less';
  const row = document.createElement('tr');
  row.className = 'review-map-strip';
  row.setAttribute('data-bullet-index', String(bullet.Index));
  const hasDetails = Boolean(bullet.Details?.length);
  const body = noteRef?.Note || bullet.Summary || '';
  row.innerHTML = `
    <td class="lines-num review-map-strip-num" aria-hidden="true"></td>
    <td colspan="5" class="lines-code review-map-strip-cell">
      <span class="review-map-strip-head">
        <span class="review-map-strip-no">${bullet.Index + 1}</span>
        <span class="review-map-strip-info${body ? '' : ' tw-hidden'}" title="${body.replace(/"/g, '&quot;')}" aria-label="${body.replace(/"/g, '&quot;')}">${tpl?.querySelector('.review-map-strip-info-icon')?.outerHTML || 'ⓘ'}</span>
      </span>
      <span class="review-map-strip-body">
        <span class="review-map-strip-title"></span>
        <span class="review-map-strip-text"></span>
        <button type="button" class="review-map-strip-more tw-hidden" data-more="${moreLabel}" data-less="${lessLabel}">${moreLabel}</button>
        <button type="button" class="review-map-strip-close" aria-label="${tpl?.getAttribute('data-close') || 'Close'}">×</button>
      </span>
      <span class="review-map-strip-details tw-hidden"></span>
    </td>`;
  row.querySelector('.review-map-strip-title')!.textContent = bullet.Text;
  row.querySelector('.review-map-strip-text')!.textContent = body;
  const more = row.querySelector<HTMLElement>('.review-map-strip-more');
  if (hasDetails) {
    more?.classList.remove('tw-hidden');
    const details = row.querySelector('.review-map-strip-details');
    if (details) details.textContent = (bullet.Details || []).join(' ');
  }
  return row;
}

// setActiveBullet marks the sidebar card of the applied bullet; pass null to clear.
export function setActiveBullet(panel: HTMLElement, index: number | null): void {
  for (const bullet of panel.querySelectorAll('.review-map-bullet')) {
    bullet.classList.toggle('review-map-active', bullet.getAttribute('data-bullet-index') === String(index));
  }
}

// nextUnapprovedBulletIndex returns the first bullet (by panel order) whose valid refs are not
// all approved yet, skipping the given index; -1 when every bullet is done.
export function nextUnapprovedBulletIndex(approval: {approved: number, total: number}[], skipIndex: number, flagged?: Set<number>): number {
  for (const [i, a] of approval.entries()) {
    if (i !== skipIndex && !flagged?.has(i) && a.total > 0 && a.approved < a.total) return i;
  }
  return -1;
}

// collapseReviewMapFences hides the raw ```review-map block behind a small toggle so the
// rendered description stays readable; the data remains one click away. Runs on any page
// whose rendered markdown contains the fenced block (PR description, comments).
export function collapseReviewMapFences(root: Document | HTMLElement = document): void {
  for (const code of Array.from(root.querySelectorAll('pre code.language-review-map'))) {
    const pre = code.closest('pre');
    if (!pre || pre.previousElementSibling?.classList.contains('review-map-fence-toggle')) continue;
    const toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.className = 'btn interact-fg review-map-fence-toggle';
    toggle.setAttribute('aria-expanded', 'false');
    toggle.textContent = 'review-map ▸';
    pre.before(toggle);
    pre.classList.add('tw-hidden');
    toggle.addEventListener('click', () => {
      const open = pre.classList.toggle('tw-hidden');
      toggle.setAttribute('aria-expanded', String(!open));
      toggle.textContent = open ? 'review-map ▸' : 'review-map ▾';
    });
  }
}

export function initReviewMapPanel() {
  collapseReviewMapFences();
  // conversation page: the summary lives collapsed inside the PR's first comment
  const convToggle = document.querySelector<HTMLElement>('#review-map-conv-toggle');
  const convStrip = document.querySelector<HTMLElement>('#review-map-conv-strip');
  if (convToggle && convStrip) {
    const convOpenKey = 'review-map-conv-open';
    const setConvOpen = (open: boolean) => {
      convStrip.classList.toggle('tw-hidden', !open);
      convToggle.setAttribute('aria-expanded', String(open));
      localUserSettings.setBoolean(convOpenKey, open);
    };
    convToggle.addEventListener('click', () => setConvOpen(convStrip.classList.contains('tw-hidden')));
    if (localUserSettings.getBoolean(convOpenKey, false)) setConvOpen(true);
  }

  const panel = document.querySelector<HTMLElement>('#review-map-panel');
  if (!panel) return;
  const data = window.config.pageData.reviewMap;
  if (!data) return;

  addDelegatedEventListener<HTMLInputElement, Event>(panel, 'change', '.review-map-check', (checkbox) => {
    const bullet = checkbox.closest('.review-map-bullet') as HTMLElement;
    // fire-and-forget: the checkbox keeps its clicked state, progress counts refresh on the next page load
    POST(data.toggleUrl, {data: {hash: bullet.getAttribute('data-bullet-hash'), reviewed: checkbox.checked}});
  });

  // interactive controls inside a card must not trigger the card's navigation/filter click
  const cardControls = '.review-map-check, .review-map-ref-check, .review-map-approve-chip, .review-map-flag-btn, .review-map-comment';

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
  // notify other modules (the file tree narrows to the visible boxes)
  // strips always exist; the selected change's strips expand, all others collapse
  const setStripsOpen = (index: number | null) => {
    for (const strip of document.querySelectorAll('tr.review-map-strip')) {
      strip.classList.toggle('review-map-strip-open', strip.getAttribute('data-bullet-index') === String(index));
    }
  };
  addDelegatedEventListener<HTMLElement, MouseEvent>(document, 'click', 'tr.review-map-strip', (strip, e) => {
    const target = e.target as HTMLElement;
    if (target.closest('.review-map-strip-close')) {
      strip.classList.remove('review-map-strip-open');
      return;
    }
    const more = target.closest<HTMLElement>('.review-map-strip-more');
    if (more) {
      const open = strip.classList.toggle('review-map-strip-more-open');
      more.textContent = open ? more.getAttribute('data-less') : more.getAttribute('data-more');
      strip.querySelector('.review-map-strip-details')?.classList.toggle('tw-hidden', !open);
      return;
    }
    strip.classList.toggle('review-map-strip-open');
  });
  buildStripRows(data, document);

  // ?map=<index> deep link: the URL wins over the sessionStorage handoff, and selections
  // rewrite it so a filtered view is shareable as-is
  const setMapParam = (index: number | null) => {
    const params = new URLSearchParams(window.location.search);
    if (index === null) params.delete('map');
    else params.set('map', String(index));
    const query = params.toString();
    window.history.replaceState(null, '', `${window.location.pathname}${query ? `?${query}` : ''}`);
  };

  const notifyFilterChanged = () => {
    document.dispatchEvent(new CustomEvent('review-map:filter-changed'));
  };

  // "Approve change" next to the Review button: approves every file of the selected change
  const approveChangeButton = (() => {
    const reviewBox = document.querySelector('#review-box');
    if (!reviewBox || !data.canReview) return null;
    const tpl = panel.querySelector('#review-map-approve-change-tpl');
    const button = document.createElement('button');
    button.id = 'review-map-approve-change';
    button.type = 'button';
    button.className = 'ui tiny green button tw-mr-2';
    button.textContent = tpl?.getAttribute('data-label') || 'Approve change';
    const tooltip = tpl?.getAttribute('data-tooltip');
    if (tooltip) button.title = tooltip;
    button.setAttribute('hidden', 'hidden'); // only visible while a change is selected
    reviewBox.before(button);
    return button;
  })();

  const applyBulletByIndex = (index: number) => {
    hideElem(fallbackBox);
    const bullet = data.bullets.find((it) => it.Index === index);
    if (!bullet) return;
    if (applyBulletFilter(bullet, document)) {
      showElem(showAllButton);
      setActiveBullet(panel, index);
      setStripsOpen(index);
      sessionStorage.setItem(filterStorageKey, String(index)); // reloads keep the active filter
      setMapParam(index); // the URL mirrors the selection so the view is shareable
      document.querySelector('#diff-file-boxes')?.setAttribute('data-review-map-active', String(index));
      const visibleCount = document.querySelectorAll('#diff-file-boxes .diff-file-box:not(.tw-hidden)').length;
      showBulletStats(bullet, visibleCount, totalBoxCount);
      if (approveChangeButton) {
        approveChangeButton.removeAttribute('hidden');
        const activeChip = panel.querySelector('.review-map-bullet.review-map-active .review-map-approve-chip');
        approveChangeButton.classList.toggle('review-map-change-done', Boolean(activeChip?.classList.contains('review-map-done')));
      }
      document.querySelector('tr.review-map-line-highlight')?.scrollIntoView({block: 'center'});
    } else {
      setActiveBullet(panel, null);
      setStripsOpen(null);
      restoreStats();
      if (approveChangeButton) approveChangeButton.setAttribute('hidden', 'hidden');
      showElem(fallbackBox);
      const nameHash = bullet.Refs[0]?.NameHash;
      if (nameHash) document.querySelector(`#diff-${nameHash}`)?.scrollIntoView();
    }
    notifyFilterChanged();
  };

  addDelegatedEventListener<HTMLElement, MouseEvent>(panel, 'click', '.review-map-bullet', (bullet, e) => {
    if ((e.target as HTMLElement).closest(cardControls)) return;
    e.preventDefault();
    if (isFilesPage()) {
      applyBulletByIndex(parseInt(bullet.getAttribute('data-bullet-index')!, 10));
      return;
    }
    // conversation page and Review Map tab: let the files page apply the filter
    sessionStorage.setItem(filterStorageKey, bullet.getAttribute('data-bullet-index')!);
    window.location.assign(`${data.issueLink}/files`);
  });

  // per-file approval inside a bullet: recount the card's chip from its checkboxes
  const refreshChip = (bullet: HTMLElement) => {
    const chip = bullet.querySelector<HTMLElement>('.review-map-approve-chip');
    if (!chip) return;
    const checks = Array.from(bullet.querySelectorAll<HTMLInputElement>('.review-map-ref-check'));
    const approved = checks.filter((c) => c.checked).length;
    const all = checks.length > 0 && approved === checks.length;
    chip.setAttribute('data-approved', String(approved));
    chip.setAttribute('data-total', String(checks.length));
    chip.querySelector('.review-map-chip-count')!.textContent = `${approved}/${checks.length}`;
    chip.classList.toggle('review-map-done', all);
    chip.classList.toggle('tw-bg-green', all);
    chip.classList.toggle('tw-text-white', all);
    bullet.classList.toggle('review-map-approved', all && !bullet.classList.contains('review-map-is-flagged'));
  };

  const panelApproval = (): {approved: number, total: number}[] =>
    Array.from(panel.querySelectorAll<HTMLElement>('.review-map-bullet'), (bullet) => {
      const checks = bullet.querySelectorAll('.review-map-ref-check');
      return {approved: bullet.querySelectorAll('.review-map-ref-check:checked').length, total: checks.length};
    });

  // the reviewer's own flags (server-rendered flag buttons carry the state)
  const flaggedIndexes = (): Set<number> => new Set(
    Array.from(panel.querySelectorAll<HTMLElement>('.review-map-flag-btn.review-map-flagged'), (btn) => parseInt(btn.closest('.review-map-bullet')!.getAttribute('data-bullet-index')!, 10)),
  );

  const maybeAdvance = (bullet: HTMLElement) => {
    const index = parseInt(bullet.getAttribute('data-bullet-index')!, 10);
    const next = nextUnapprovedBulletIndex(panelApproval(), index, flaggedIndexes());
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

  // approve (or clear) every valid file reference of one change card; approving a
  // flagged change clears the flag first (they are opposite votes)
  const approveAllRefs = (bullet: HTMLElement, target: boolean) => {
    const hash = bullet.getAttribute('data-bullet-hash')!;
    const flagBtn = bullet.querySelector<HTMLElement>('.review-map-flag-btn');
    if (target && flagBtn?.classList.contains('review-map-flagged')) {
      flagBtn.classList.remove('review-map-flagged');
      POST(data.toggleUrl, {data: {hash, flag: true, reviewed: false}});
    }
    for (const check of Array.from(bullet.querySelectorAll<HTMLInputElement>('.review-map-ref-check'))) {
      if (check.checked === target) continue;
      check.checked = target;
      POST(data.toggleUrl, {data: {hash, refIndex: parseInt(check.getAttribute('data-ref-index')!, 10), reviewed: target}});
    }
    refreshChip(bullet);
  };

  addDelegatedEventListener<HTMLButtonElement, MouseEvent>(panel, 'click', '.review-map-approve-chip', (chip, e) => {
    e.preventDefault();
    const bullet = chip.closest('.review-map-bullet') as HTMLElement;
    const target = chip.getAttribute('data-approved') !== chip.getAttribute('data-total'); // approve all unless everything is approved
    approveAllRefs(bullet, target);
    if (target) maybeAdvance(bullet);
  });

  approveChangeButton?.addEventListener('click', () => {
    const active = panel.querySelector<HTMLElement>('.review-map-bullet.review-map-active');
    if (!active) return;
    const chip = active.querySelector<HTMLElement>('.review-map-approve-chip');
    const target = chip ? chip.getAttribute('data-approved') !== chip.getAttribute('data-total') : true;
    approveAllRefs(active, target);
    approveChangeButton.classList.toggle('review-map-change-done', target);
    if (target) maybeAdvance(active);
  });

  // flagging clears approvals (opposite vote), stops auto-advance past this change, and
  // opens the inline comment form at the change's first mapped line for the discussion
  addDelegatedEventListener<HTMLButtonElement, MouseEvent>(panel, 'click', '.review-map-flag-btn', (btn, e) => {
    e.preventDefault();
    const bullet = btn.closest('.review-map-bullet') as HTMLElement;
    const hash = bullet.getAttribute('data-bullet-hash')!;
    const flagging = !btn.classList.contains('review-map-flagged');
    btn.classList.toggle('review-map-flagged', flagging);
    bullet.classList.toggle('review-map-is-flagged', flagging);
    POST(data.toggleUrl, {data: {hash, flag: true, reviewed: flagging}});
    if (flagging) {
      approveAllRefs(bullet, false);
      const index = parseInt(bullet.getAttribute('data-bullet-index')!, 10);
      const firstRef = data.bullets.find((it) => it.Index === index)?.Refs.find((ref) => ref.Valid);
      const anchor = firstRef ? document.querySelector(`#diff-${firstRef.NameHash} .add-code-comment[data-side="right"][data-idx="${CSS.escape(String(firstRef.StartLine))}"]`) : null;
      (anchor as HTMLElement | null)?.click(); // no anchor on this page: nothing to open
    }
  });

  if (isFilesPage()) {
    showAllButton.addEventListener('click', () => {
      clearBulletFilter(document);
      hideElem(showAllButton);
      setStripsOpen(null);
      setActiveBullet(panel, null);
      restoreStats();
      if (approveChangeButton) approveChangeButton.setAttribute('hidden', 'hidden');
      document.querySelector('#diff-file-boxes')?.removeAttribute('data-review-map-active');
      sessionStorage.removeItem(filterStorageKey);
      setMapParam(null);
      notifyFilterChanged();
    });

    const storedIndex = sessionStorage.getItem(filterStorageKey);
    const mapParam = new URLSearchParams(window.location.search).get('map');
    const initialIndex = mapParam !== null ? parseInt(mapParam, 10) : (storedIndex !== null ? parseInt(storedIndex, 10) : null);
    if (initialIndex !== null && Number.isFinite(initialIndex)) {
      applyBulletByIndex(initialIndex); // kept until "Show all files": reloads stay filtered
    }
  }
}
