import {toggleElem} from '../utils/dom.ts';
import {localUserSettings} from '../modules/user-settings.ts';

// Horizontal drag-resize for the left diff sidebars (review map, file tree) plus the review
// map sidebar's hide/show toggle. Widths and the hidden state persist per browser.
const treeWidthKey = 'diff-file-tree-width';
const mapWidthKey = 'review-map-width';
const mapHiddenKey = 'review-map-hidden';

function applyWidth(sidebar: HTMLElement, width: number, min: number, max: number): void {
  const clamped = Math.min(max, Math.max(min, Math.round(width)));
  if (sidebar.id === 'diff-file-tree') {
    sidebar.style.flexBasis = `${clamped}px`; // css keeps flex-basis 20% + max-width 380px
    sidebar.style.maxWidth = `${clamped}px`;
  } else {
    sidebar.style.width = `${clamped}px`;
  }
}

function enableHorizontalResize(sidebar: HTMLElement, storageKey: string, min: number, max: number): void {
  const stored = parseInt(localUserSettings.getString(storageKey), 10);
  if (Number.isFinite(stored)) applyWidth(sidebar, stored, min, max);

  const handle = document.createElement('div');
  handle.classList.add('diff-sidebar-resizer');
  handle.setAttribute('role', 'separator');
  handle.setAttribute('aria-orientation', 'vertical');
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
}

export function initDiffSidebars() {
  if (!document.querySelector('#diff-file-boxes')) return;
  const tree = document.querySelector<HTMLElement>('#diff-file-tree');
  if (tree) enableHorizontalResize(tree, treeWidthKey, 180, Math.min(560, window.innerWidth * 0.4));
  const map = document.querySelector<HTMLElement>('.review-map-sidebar');
  if (map) enableHorizontalResize(map, mapWidthKey, 240, Math.min(860, window.innerWidth * 0.5));

  const hideBtn = map?.querySelector<HTMLElement>('#review-map-hide');
  const showBtn = document.querySelector<HTMLElement>('#review-map-show');
  if (!map || !hideBtn || !showBtn) return;
  const setHidden = (hidden: boolean) => {
    toggleElem(map, !hidden);
    toggleElem(showBtn, hidden);
    localUserSettings.setBoolean(mapHiddenKey, hidden);
  };
  hideBtn.addEventListener('click', () => setHidden(true));
  showBtn.addEventListener('click', () => setHidden(false));
  if (localUserSettings.getBoolean(mapHiddenKey, false)) {
    toggleElem(map, false);
    toggleElem(showBtn, true);
  }
}
