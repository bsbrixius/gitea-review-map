import {toggleElem} from '../utils/dom.ts';
import {localUserSettings} from '../modules/user-settings.ts';

// Horizontal drag-resize for the left diff sidebars (review map, file tree), the review
// map sidebar's hide/show toggle, and the file-tree narrowing while a review-map change
// is selected. Widths, the hidden state and the tree mode persist per browser.
const treeWidthKey = 'diff-file-tree-width';
const mapWidthKey = 'review-map-width';
const mapHiddenKey = 'review-map-hidden';
const treeDimKey = 'review-map-tree-dim-others'; // true: dim non-change files, false (default): hide them

function applyWidth(sidebar: HTMLElement, width: number, min: number, max: number): void {
  const clamped = Math.min(max, Math.max(min, Math.round(width)));
  if (sidebar.id === 'diff-file-tree') {
    sidebar.style.flexBasis = `${clamped}px`; // css keeps flex-basis 20% + max-width 380px
    sidebar.style.maxWidth = `${clamped}px`;
  } else {
    sidebar.style.width = `${clamped}px`;
  }
}

// widest text in the sidebar, measured with wrapping and clamping disabled
function naturalTextWidth(sidebar: HTMLElement): number {
  const probe = document.createElement('span');
  probe.style.cssText = 'position:absolute;visibility:hidden;white-space:nowrap;left:-9999px;top:0';
  let widest = 0;
  const texts = sidebar.querySelectorAll('.review-map-bullet-text, .review-map-ref, .item-file .gt-ellipsis, .item-directory .gt-ellipsis');
  for (const el of texts) {
    probe.style.font = getComputedStyle(el).font;
    probe.textContent = el.textContent;
    document.body.append(probe);
    widest = Math.max(widest, probe.getBoundingClientRect().width);
    probe.remove();
  }
  return widest;
}

function fitToContent(sidebar: HTMLElement, storageKey: string, min: number, max: number, allowance: number): void {
  applyWidth(sidebar, naturalTextWidth(sidebar) + allowance, min, max);
  localUserSettings.setString(storageKey, String(Math.round(sidebar.getBoundingClientRect().width)));
}

function enableHorizontalResize(sidebar: HTMLElement, storageKey: string, min: number, max: number, allowance: number): void {
  const stored = parseInt(localUserSettings.getString(storageKey), 10);
  if (Number.isFinite(stored)) applyWidth(sidebar, stored, min, max);

  const handle = document.createElement('div');
  handle.classList.add('diff-sidebar-resizer');
  handle.setAttribute('role', 'separator');
  handle.setAttribute('aria-orientation', 'vertical');
  handle.title = sidebar.id === 'diff-file-tree' ? 'Double-click to fit' : 'Double-click to fit the texts';
  sidebar.prepend(handle);

  handle.addEventListener('pointerdown', (e: PointerEvent) => {
    if (!window.matchMedia('(min-width: 768px)').matches) return; // sidebars stack on mobile
    e.preventDefault();
    handle.setPointerCapture(e.pointerId);
    const startPointerX = e.clientX;
    const startWidth = sidebar.getBoundingClientRect().width;
    const onMove = (ev: PointerEvent) => {
      applyWidth(sidebar, startWidth + (ev.clientX - startPointerX), min, max);
    };
    const onUp = () => {
      handle.removeEventListener('pointermove', onMove);
      localUserSettings.setString(storageKey, String(Math.round(sidebar.getBoundingClientRect().width)));
    };
    handle.addEventListener('pointermove', onMove);
    handle.addEventListener('pointerup', onUp, {once: true});
  });

  handle.addEventListener('dblclick', () => {
    if (window.matchMedia('(min-width: 768px)').matches) fitToContent(sidebar, storageKey, min, max, allowance);
  });
}

// --- file-tree narrowing while a review-map change is selected -----------------

// The Vue file tree renders FLAT: a directory row (.item-directory) is followed by its
// .sub-items sibling, and file rows are a.item-file anchors. markTreeList walks one such
// list and returns whether every row in it is fully outside the selected change.
function markTreeList(container: Element, visible: Set<string>): boolean {
  const children = Array.from(container.children);
  let allExcluded = true, hasRows = false;
  for (const [i, child] of children.entries()) {
    if (child.classList.contains('diff-sidebar-resizer')) continue; // our own resize handle
    hasRows = true;
    if (child.classList.contains('item-file')) {
      const hash = (child.getAttribute('href') || '').replace('#diff-', '');
      const excluded = !visible.has(hash);
      child.classList.toggle('review-map-tree-excluded', excluded);
      if (!excluded) allExcluded = false;
    } else if (child.classList.contains('item-directory')) {
      const sub = children[i + 1]?.classList.contains('sub-items') ? children[i + 1] : null;
      const subAllExcluded = sub ? markTreeList(sub, visible) : false;
      child.classList.toggle('review-map-tree-excluded', subAllExcluded);
      if (!subAllExcluded) allExcluded = false;
    } else if (child.classList.contains('sub-items')) {
       // consumed together with its directory row
    } else if (!markTreeList(child, visible)) { // unknown wrapper: descend
      allExcluded = false;
    }
  }
  return hasRows && allExcluded;
}

// applyTreeFilter narrows the file tree to the visible diff boxes (by their #diff-<hash>
// anchors). An empty set clears the filter. Whether excluded rows are hidden or dimmed
// depends on the "show non-change files dimmed" option; both classes are always applied
// so switching the option only rewrites the tree root class.
export function applyTreeFilter(tree: HTMLElement, visibleHashes: Set<string> | null): void {
  const dimOthers = localUserSettings.getBoolean(treeDimKey, false);
  tree.classList.toggle('review-map-tree-dim', dimOthers);
  tree.classList.toggle('review-map-tree-hide', !dimOthers);
  for (const el of tree.querySelectorAll('.review-map-tree-excluded')) {
    el.classList.remove('review-map-tree-excluded');
  }
  if (!visibleHashes || visibleHashes.size === 0) {
    tree.classList.remove('review-map-tree-dim', 'review-map-tree-hide');
    return;
  }
  markTreeList(tree, visibleHashes);
}

function initTreeNarrowing(tree: HTMLElement): void {
  // the Vue tree re-renders on navigation; keep an active filter applied
  let applyFromDom: () => void;
  const observer = new MutationObserver(() => {
    if (tree.classList.contains('review-map-tree-hide') || tree.classList.contains('review-map-tree-dim')) applyFromDom();
  });
  applyFromDom = () => {
    const boxes = Array.from(document.querySelectorAll('#diff-file-boxes .diff-file-box:not(.tw-hidden)'));
    const hashes = new Set(boxes.map((box) => (box.id || '').replace('diff-', '')));
    // disconnect while marking: our own class changes must not re-trigger the observer
    observer.disconnect();
    applyTreeFilter(tree, boxes.length ? hashes : null);
    observer.observe(tree, {childList: true, subtree: true});
  };
  document.addEventListener('review-map:filter-changed', applyFromDom);
  observer.observe(tree, {childList: true, subtree: true});

  const toggle = document.querySelector<HTMLInputElement>('#review-map-tree-toggle');
  if (toggle) {
    toggle.checked = localUserSettings.getBoolean(treeDimKey, false);
    toggle.addEventListener('click', (e) => {
      e.stopPropagation(); // keep the Diff Options dropdown open while toggling
    });
    toggle.addEventListener('change', () => {
      localUserSettings.setBoolean(treeDimKey, toggle.checked);
      applyFromDom();
    });
  }
}

export function initDiffSidebars() {
  if (!document.querySelector('#diff-file-boxes')) return;
  const tree = document.querySelector<HTMLElement>('#diff-file-tree');
  if (tree) {
    enableHorizontalResize(tree, treeWidthKey, 180, Math.min(560, window.innerWidth * 0.4), 70);
    initTreeNarrowing(tree);
  }
  const map = document.querySelector<HTMLElement>('.review-map-sidebar');
  if (map) enableHorizontalResize(map, mapWidthKey, 240, Math.min(860, window.innerWidth * 0.5), 90);

  // file-tree-style toggle button (rendered in the diff header next to the tree toggle)
  const toggleBtn = document.querySelector<HTMLElement>('#review-map-toggle');
  if (!map || !toggleBtn) return;
  const setHidden = (hidden: boolean) => {
    toggleElem(map, !hidden);
    // same reversed-icon convention as the file tree button: visible shows "expand"
    toggleBtn.querySelector('.octicon-sidebar-expand')?.classList.toggle('tw-hidden', hidden);
    toggleBtn.querySelector('.octicon-sidebar-collapse')?.classList.toggle('tw-hidden', !hidden);
    toggleBtn.setAttribute('data-tooltip-content', toggleBtn.getAttribute(hidden ? 'data-show-text' : 'data-hide-text') || '');
    localUserSettings.setBoolean(mapHiddenKey, hidden);
  };
  toggleBtn.addEventListener('click', () => setHidden(!map.classList.contains('tw-hidden')));
  // always run once: it also unhides the state icon (the template ships with both hidden)
  setHidden(localUserSettings.getBoolean(mapHiddenKey, false));
}
