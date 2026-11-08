import {applyBulletFilter, buildStripRows, clearBulletFilter, initReviewMapPanel, isFilesPage, nextUnapprovedBulletIndex, setActiveBullet} from './review-map.ts';
import {applyTreeFilter} from './diff-sidebars.ts';
import {localUserSettings} from '../modules/user-settings.ts';
import {POST} from '../modules/fetch.ts';

vi.mock('../modules/fetch.ts', () => ({
  POST: vi.fn(),
}));

// the fixture mirrors the locked DOM contract: templates/repo/diff/box.tmpl wraps
// ".diff-file-box[data-new-filename][data-old-filename]" elements with "table.chroma[data-path]"
// rows inside; templates/repo/diff/section_split.tmpl renders tr.{add,del,same,tag}-code rows with
// "td.lines-num-new[data-line-num]" (the attribute is empty for deletion rows without a new-side match)
const diffHTML = `
<div id="diff-file-boxes">
  <div class="diff-file-box file-content" id="diff-aaaa" data-old-filename="src/a.txt" data-new-filename="src/a.txt">
    <table class="chroma" data-path="src/a.txt">
      <tr class="tag-code" data-line-type="tag" id="a-tag-1">
        <td class="lines-num lines-num-old"></td>
        <td colspan="6" class="lines-code lines-code-old">@@ -1,5 +1,5 @@</td>
      </tr>
      <tr class="same-code" data-line-type="same" id="a-ctx-1">
        <td class="lines-num lines-num-old" data-line-num="1"></td>
        <td class="lines-num lines-num-new" data-line-num="1"></td>
      </tr>
      <tr class="add-code" data-line-type="add" id="a-add-2">
        <td class="lines-num lines-num-new" data-line-num="2"></td>
      </tr>
      <tr class="add-code" data-line-type="add" id="a-add-3">
        <td class="lines-num lines-num-new" data-line-num="3"></td>
      </tr>
      <tr class="add-comment" data-line-type="add" id="a-comment-3">
        <td class="add-comment-left" colspan="4"></td>
        <td class="add-comment-right" colspan="4"></td>
      </tr>
      <tr class="same-code" data-line-type="same" id="a-ctx-4">
        <td class="lines-num lines-num-new" data-line-num="4"></td>
      </tr>
      <tr class="del-code" data-line-type="del" id="a-del-9">
        <td class="lines-num lines-num-old" data-line-num="9"></td>
        <td class="lines-num lines-num-new" data-line-num=""></td>
      </tr>
      <tr class="tag-code" data-line-type="tag" id="a-tag-2">
        <td class="lines-num lines-num-old"></td>
        <td colspan="6" class="lines-code lines-code-old">@@ -20,3 +20,3 @@</td>
      </tr>
      <tr class="same-code" data-line-type="same" id="a-ctx-20">
        <td class="lines-num lines-num-new" data-line-num="20"></td>
      </tr>
      <tr class="del-code" data-line-type="del" id="a-del-21">
        <td class="lines-num lines-num-old" data-line-num="21"></td>
        <td class="lines-num lines-num-new" data-line-num=""></td>
      </tr>
      <tr class="tag-code" data-line-type="tag" id="a-tag-3">
        <td class="lines-num lines-num-old"></td>
        <td colspan="6" class="lines-code lines-code-old">@@ -30,2 +30,2 @@</td>
      </tr>
    </table>
  </div>
  <div class="diff-file-box file-content" id="diff-bbbb" data-old-filename="docs/other.md" data-new-filename="docs/other.md">
    <table class="chroma" data-path="docs/other.md">
      <tr class="tag-code" data-line-type="tag" id="b-tag-1"></tr>
      <tr class="add-code" data-line-type="add" id="b-add-1">
        <td class="lines-num lines-num-new" data-line-num="1"></td>
      </tr>
    </table>
  </div>
</div>`;

const panelHTML = `
<div id="review-map-panel" data-issue-link="/owner/repo/pulls/1">
  <div class="review-map-bullets">
    <div class="review-map-bullet" data-bullet-index="0" data-bullet-hash="hash-a">
      <input type="checkbox" class="review-map-check">
      <div>
        <span class="review-map-bullet-text">Add the feature</span>
        <input type="checkbox" class="review-map-ref-check" data-ref-index="0">
        <button type="button" class="review-map-approve-chip" data-approved="0" data-total="1"><span class="review-map-chip-count">0/1</span></button>
        <button type="button" class="review-map-flag-btn">flag</button>
        <a class="review-map-ref" href="/owner/repo/pulls/1/files" data-ref-file="src/a.txt" data-ref-start="3" data-ref-end="4" data-name-hash="aaaa">src/a.txt:3-4</a>
        <a class="review-map-comment" href="/owner/repo/pulls/1/files#issuecomment-1">#</a>
      </div>
    </div>
    <div class="review-map-bullet" data-bullet-index="1" data-bullet-hash="hash-b">
      <div>
        <span class="review-map-bullet-text">Fix the bug</span>
        <button type="button" class="review-map-flag-btn">flag</button>
        <a class="review-map-ref" href="/owner/repo/pulls/1/files" data-ref-file="docs/other.md" data-ref-start="1" data-ref-end="1" data-name-hash="bbbb">docs/other.md:1-1</a>
      </div>
    </div>
  </div>
  <button id="review-map-show-all" type="button" class="tw-hidden"></button>
  <div id="review-map-fallback" class="tw-hidden"></div>
</div>`;

const bulletA = {
  Index: 0, Hash: 'hash-a', Text: 'Add the feature', Orphan: false, AddedLines: 2, RemovedLines: 1, Summary: '', Details: null,
  Refs: [{File: 'src/a.txt', NameHash: 'aaaa', StartLine: 3, EndLine: 4, Valid: true, Note: ''}],
};
const bulletAStrips = {...bulletA, Summary: 'Change-level text A.', Details: ['detail one']};
const bulletB = {
  Index: 1, Hash: 'hash-b', Text: 'Fix the bug', Orphan: false, AddedLines: 2, RemovedLines: 1, Summary: '', Details: null,
  Refs: [{File: 'docs/other.md', NameHash: 'bbbb', StartLine: 1, EndLine: 1, Valid: true, Note: 'File note B.'}],
};
const bulletBStrips = {...bulletB, Summary: '', Details: null, Refs: [...bulletB.Refs, {File: 'src/a.txt', NameHash: 'aaaa', StartLine: 20, EndLine: 20, Valid: true, Note: ''}]};

const pageData = {
  ignoredReason: '',
  issueLink: '/owner/repo/pulls/1',
  toggleUrl: '/owner/repo/pulls/1/review-map/progress',
  canReview: true,
  myReviewed: [],
  myApprovedRefs: [],
  bulletApproval: [{approved: 0, total: 1}, {approved: 0, total: 1}],
  myFlagged: [],
  bulletFlags: [0, 0],
  flaggedCount: 0,
  bullets: [bulletA, bulletB],
  commentLinks: {},
  coverage: {TotalChanged: 6, MappedChanged: 5, UnmappedFiles: ['docs/other.md']},
  ignoreGlobs: [],
  invalidRefCount: 0,
  orphanCount: 0,
  progress: [],
};

function stripsPageData() {
  return {...pageData, bullets: [bulletAStrips, bulletBStrips]};
}

function isHidden(el: Element): boolean {
  return el.classList.contains('tw-hidden');
}

beforeEach(() => {
  document.body.innerHTML = '';
  sessionStorage.clear();
  window.config.pageData.reviewMap = pageData;
  vi.clearAllMocks();
});

afterEach(() => {
  sessionStorage.clear(); // the whole suite shares one environment (isolate: false)
});

// the tests share the document and the sessionStorage of the happy-dom environment,
// so they must not run under the config's "sequence.concurrent" mode
describe('review-map', {concurrent: false}, () => {
  describe('filter', () => {
    test('isFilesPage follows the diff-file-boxes container', () => {
      document.body.innerHTML = diffHTML;
      expect(isFilesPage()).toBeTruthy();
      document.querySelector('#diff-file-boxes')!.remove();
      expect(isFilesPage()).toBeFalsy();
    });

    test('hides the unmatched file box, keeps the matched one', () => {
      document.body.innerHTML = diffHTML;
      expect(applyBulletFilter(bulletA, document)).toBeTruthy();
      expect(isHidden(document.querySelector('#diff-aaaa')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#diff-bbbb')!)).toBeTruthy();
    });

    test('hides rows outside the ref range, shows and highlights rows inside it', () => {
      document.body.innerHTML = diffHTML;
      expect(applyBulletFilter(bulletA, document)).toBeTruthy();
      expect(isHidden(document.querySelector('#a-ctx-1')!)).toBeTruthy(); // line 1 outside 3-4
      expect(isHidden(document.querySelector('#a-add-2')!)).toBeTruthy(); // line 2 outside 3-4
      expect(isHidden(document.querySelector('#a-add-3')!)).toBeFalsy(); // line 3 inside
      expect(isHidden(document.querySelector('#a-ctx-4')!)).toBeFalsy(); // line 4 inside
      expect(document.querySelector('#a-add-3')!.classList.contains('review-map-line-highlight')).toBeTruthy();
      expect(document.querySelector('#a-ctx-4')!.classList.contains('review-map-line-highlight')).toBeTruthy();
      expect(document.querySelector('#a-ctx-1')!.classList.contains('review-map-line-highlight')).toBeFalsy();
    });

    test('anchors deletion rows without a new-side number on the last seen new-side number', () => {
      document.body.innerHTML = diffHTML;
      expect(applyBulletFilter(bulletA, document)).toBeTruthy();
      // previous row is line 4, inside 3-4, so the deletion row is shown and highlighted
      expect(isHidden(document.querySelector('#a-del-9')!)).toBeFalsy();
      expect(document.querySelector('#a-del-9')!.classList.contains('review-map-line-highlight')).toBeTruthy();
      // previous row is line 20, outside 3-4, so the deletion row is hidden
      expect(isHidden(document.querySelector('#a-del-21')!)).toBeTruthy();
    });

    test('keeps a hunk header when a following row is shown, hides it otherwise', () => {
      document.body.innerHTML = diffHTML;
      expect(applyBulletFilter(bulletA, document)).toBeTruthy();
      expect(isHidden(document.querySelector('#a-tag-1')!)).toBeFalsy(); // first hunk contains line 3
      expect(isHidden(document.querySelector('#a-tag-2')!)).toBeTruthy(); // second hunk has no visible row
      expect(isHidden(document.querySelector('#a-tag-3')!)).toBeTruthy(); // trailing hunk has no code row
    });

    test('comment rows inherit the visibility of their code row', () => {
      document.body.innerHTML = diffHTML;
      expect(applyBulletFilter(bulletA, document)).toBeTruthy();
      expect(isHidden(document.querySelector('#a-comment-3')!)).toBeFalsy(); // follows visible line 3
    });

    test('applying another bullet filter retargets in place', () => {
      document.body.innerHTML = diffHTML;
      expect(applyBulletFilter(bulletA, document)).toBeTruthy();
      expect(isHidden(document.querySelector('#diff-bbbb')!)).toBeTruthy();
      expect(applyBulletFilter(bulletB, document)).toBeTruthy();
      expect(isHidden(document.querySelector('#diff-aaaa')!)).toBeTruthy();
      expect(isHidden(document.querySelector('#diff-bbbb')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#b-add-1')!)).toBeFalsy();
      expect(document.querySelector('#a-add-3')!.classList.contains('review-map-line-highlight')).toBeFalsy();
    });

    test('clearBulletFilter restores boxes, rows and highlights', () => {
      document.body.innerHTML = diffHTML;
      expect(applyBulletFilter(bulletA, document)).toBeTruthy();
      clearBulletFilter(document);
      for (const el of document.querySelectorAll('#diff-file-boxes .diff-file-box, #diff-file-boxes tr')) {
        expect(isHidden(el)).toBeFalsy();
      }
      expect(document.querySelectorAll('.review-map-line-highlight')).toHaveLength(0);
    });

    test('returns false and leaves the diff untouched when no file box matches', () => {
      document.body.innerHTML = diffHTML;
      const missing = {Index: 2, Hash: 'hash-c', Text: 'Elsewhere', Orphan: true, AddedLines: 2, RemovedLines: 1, Summary: '', Details: null, Refs: [{File: 'missing.txt', NameHash: 'cccc', StartLine: 1, EndLine: 2, Valid: true, Note: ''}]};
      expect(applyBulletFilter(missing, document)).toBeFalsy();
      for (const el of document.querySelectorAll('#diff-file-boxes .diff-file-box, #diff-file-boxes tr')) {
        expect(isHidden(el)).toBeFalsy();
      }
    });
  });

  describe('panel init', () => {
    test('conversation page: clicking a bullet stores the filter and navigates to the files page', () => {
      document.body.innerHTML = panelHTML;
      const spyAssign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
      initReviewMapPanel();
      (document.querySelector('.review-map-bullet-text') as HTMLElement).click();
      expect(sessionStorage.getItem('review-map-filter')).toEqual('0');
      expect(spyAssign).toHaveBeenCalledWith('/owner/repo/pulls/1/files');
    });

    test('conversation page: clicking a ref link stores the filter for the whole bullet', () => {
      document.body.innerHTML = panelHTML;
      const spyAssign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
      initReviewMapPanel();
      (document.querySelector('.review-map-ref') as HTMLElement).click();
      expect(sessionStorage.getItem('review-map-filter')).toEqual('0');
      expect(spyAssign).toHaveBeenCalledWith('/owner/repo/pulls/1/files');
    });

    test('conversation page: the checkbox and comment links do not navigate', () => {
      document.body.innerHTML = panelHTML;
      const spyAssign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
      initReviewMapPanel();
      (document.querySelector('.review-map-check') as HTMLElement).click();
      (document.querySelector('.review-map-comment') as HTMLElement).click();
      expect(sessionStorage.getItem('review-map-filter')).toBeNull();
      expect(spyAssign).not.toHaveBeenCalled();
    });

    test('checkbox change posts the bullet hash and state', () => {
      document.body.innerHTML = panelHTML;
      initReviewMapPanel();
      const checkbox = document.querySelector<HTMLInputElement>('.review-map-check')!;
      checkbox.checked = true;
      checkbox.dispatchEvent(new Event('change', {bubbles: true}));
      expect(POST).toHaveBeenCalledWith('/owner/repo/pulls/1/review-map/progress', {data: {hash: 'hash-a', reviewed: true}});
      checkbox.checked = false;
      checkbox.dispatchEvent(new Event('change', {bubbles: true}));
      expect(POST).toHaveBeenCalledWith('/owner/repo/pulls/1/review-map/progress', {data: {hash: 'hash-a', reviewed: false}});
    });

    test('files page: a stored filter index persists until show-all clears it', () => {
      document.body.innerHTML = panelHTML + diffHTML;
      sessionStorage.setItem('review-map-filter', '0');
      initReviewMapPanel();
      expect(sessionStorage.getItem('review-map-filter')).toBe('0'); // reloads stay filtered
      expect(isHidden(document.querySelector('#diff-bbbb')!)).toBeTruthy();
      expect(isHidden(document.querySelector('#a-add-3')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#review-map-show-all')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#review-map-fallback')!)).toBeTruthy();
      (document.querySelector('#review-map-show-all') as HTMLElement).click();
      expect(sessionStorage.getItem('review-map-filter')).toBeNull();
    });

    test('files page: clicking a bullet applies the filter in place, show-all clears it', () => {
      document.body.innerHTML = panelHTML + diffHTML;
      initReviewMapPanel();
      (document.querySelector('[data-bullet-index="1"] .review-map-bullet-text') as HTMLElement).click();
      expect(isHidden(document.querySelector('#diff-aaaa')!)).toBeTruthy();
      expect(isHidden(document.querySelector('#diff-bbbb')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#review-map-show-all')!)).toBeFalsy();
      (document.querySelector('#review-map-show-all') as HTMLElement).click();
      expect(isHidden(document.querySelector('#diff-aaaa')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#a-add-3')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#review-map-show-all')!)).toBeTruthy();
    });

    test('files page: a bullet whose refs match no file box shows the fallback and scrolls to the first ref anchor', () => {
      document.body.innerHTML = panelHTML + diffHTML;
      const missing = {
        Index: 0, Hash: 'hash-a', Text: 'Add the feature', Orphan: false, AddedLines: 2, RemovedLines: 1, Summary: '', Details: null,
        Refs: [{File: 'missing.txt', NameHash: 'bbbb', StartLine: 1, EndLine: 2, Valid: true, Note: ''}],
      };
      window.config.pageData.reviewMap = {...pageData, bullets: [missing]};
      const spyScroll = vi.spyOn(document.querySelector('#diff-bbbb')!, 'scrollIntoView').mockImplementation(() => {});
      initReviewMapPanel();
      (document.querySelector('[data-bullet-index="0"] .review-map-bullet-text') as HTMLElement).click();
      expect(isHidden(document.querySelector('#review-map-fallback')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#review-map-show-all')!)).toBeTruthy();
      expect(spyScroll).toHaveBeenCalled();
      spyScroll.mockRestore();
    });

    test('files page: a matched file box whose rows all fall outside the range still filters successfully', () => {
      document.body.innerHTML = panelHTML + diffHTML;
      const retargeted = {
        Index: 0, Hash: 'hash-a', Text: 'Add the feature', Orphan: false, AddedLines: 2, RemovedLines: 1, Summary: '', Details: null,
        Refs: [{File: 'src/a.txt', NameHash: 'aaaa', StartLine: 100, EndLine: 200, Valid: true, Note: ''}],
      };
      window.config.pageData.reviewMap = {...pageData, bullets: [retargeted]};
      initReviewMapPanel();
      (document.querySelector('[data-bullet-index="0"] .review-map-bullet-text') as HTMLElement).click();
      // the box matches, so the filter applies (with every row hidden), and the fallback is not used
      expect(isHidden(document.querySelector('#diff-aaaa')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#a-add-3')!)).toBeTruthy();
      expect(isHidden(document.querySelector('#review-map-show-all')!)).toBeFalsy();
      expect(isHidden(document.querySelector('#review-map-fallback')!)).toBeTruthy();
    });

    test('no panel on the page: init does nothing', () => {
      document.body.innerHTML = diffHTML;
      initReviewMapPanel();
      expect(isHidden(document.querySelector('#diff-aaaa')!)).toBeFalsy();
    });
  });
});

describe('review map sidebar activation', () => {
  test('setActiveBullet marks only the applied bullet and clears with null', () => {
    document.body.innerHTML = panelHTML;
    const panel = document.querySelector('#review-map-panel') as HTMLElement;
    setActiveBullet(panel, 1);
    expect(document.querySelector('[data-bullet-index="0"]')!.classList.contains('review-map-active')).toBeFalsy();
    expect(document.querySelector('[data-bullet-index="1"]')!.classList.contains('review-map-active')).toBeTruthy();
    setActiveBullet(panel, null);
    expect(panel.querySelector('.review-map-active')).toBeFalsy();
  });

  test('files page: applying a bullet activates its sidebar card and show-all clears it', () => {
    document.body.innerHTML = panelHTML + diffHTML;
    window.config.pageData.reviewMap = pageData;
    initReviewMapPanel();
    (document.querySelector('[data-bullet-index="1"] .review-map-bullet-text') as HTMLElement).click();
    expect(document.querySelector('[data-bullet-index="1"]')!.classList.contains('review-map-active')).toBeTruthy();
    (document.querySelector('#review-map-show-all') as HTMLElement).click();
    expect(document.querySelector('.review-map-active')).toBeFalsy();
  });
});

describe('review map approve flow', () => {
  test('nextUnapprovedBulletIndex skips complete bullets and the current one', () => {
    const approval = [
      {approved: 2, total: 2}, // done
      {approved: 1, total: 3}, // next candidate
      {approved: 0, total: 0}, // no valid refs: never a candidate
      {approved: 0, total: 4},
    ];
    expect(nextUnapprovedBulletIndex(approval, 3)).toBe(1);
    expect(nextUnapprovedBulletIndex(approval, 1)).toBe(3);
    expect(nextUnapprovedBulletIndex([{approved: 1, total: 1}], 0)).toBe(-1);
  });
});

describe('file tree narrowing', () => {
  // real Vue tree shape: a flat list where a directory row is followed by its .sub-items sibling
  const treeHTML = `
<div id="diff-file-tree">
  <div class="diff-file-tree-items">
    <a class="item-file" href="#diff-aaaa">a.ts</a>
    <div class="item-directory">src</div>
    <div class="sub-items">
      <a class="item-file" href="#diff-bbbb">b.ts</a>
      <a class="item-file" href="#diff-cccc">c.ts</a>
    </div>
  </div>
</div>`;

  test('hide mode (default): non-change files and fully-excluded directories disappear', () => {
    document.body.innerHTML = treeHTML;
    const tree = document.querySelector('#diff-file-tree') as HTMLElement;
    applyTreeFilter(tree, new Set(['bbbb']));
    expect(tree.classList.contains('review-map-tree-hide')).toBeTruthy();
    expect(document.querySelector('[href="#diff-aaaa"]')!.classList.contains('review-map-tree-excluded')).toBeTruthy();
    expect(document.querySelector('[href="#diff-bbbb"]')!.classList.contains('review-map-tree-excluded')).toBeFalsy();
    expect(document.querySelector('[href="#diff-cccc"]')!.classList.contains('review-map-tree-excluded')).toBeTruthy();
    // b.ts inside src/ is part of the change: the directory stays visible
    expect(document.querySelector('.item-directory')!.classList.contains('review-map-tree-excluded')).toBeFalsy();
    // a change that touches nothing under src/ excludes the directory too
    applyTreeFilter(tree, new Set(['dddd']));
    expect(document.querySelector('.item-directory')!.classList.contains('review-map-tree-excluded')).toBeTruthy();
  });

  test('dim mode: excluded rows keep the class and the tree root switches mode class', () => {
    document.body.innerHTML = treeHTML;
    const tree = document.querySelector('#diff-file-tree') as HTMLElement;
    window.config.pageData = window.config.pageData ?? {};
    localUserSettings.setBoolean('review-map-tree-dim-others', true);
    applyTreeFilter(tree, new Set(['bbbb', 'cccc']));
    expect(tree.classList.contains('review-map-tree-dim')).toBeTruthy();
    expect(tree.classList.contains('review-map-tree-hide')).toBeFalsy();
    expect(document.querySelector('.item-directory')!.classList.contains('review-map-tree-excluded')).toBeFalsy();
    localUserSettings.setBoolean('review-map-tree-dim-others', false);
  });

  test('null set clears every mark and both mode classes', () => {
    document.body.innerHTML = treeHTML;
    const tree = document.querySelector('#diff-file-tree') as HTMLElement;
    applyTreeFilter(tree, new Set(['bbbb']));
    applyTreeFilter(tree, null);
    expect(tree.querySelectorAll('.review-map-tree-excluded')).toHaveLength(0);
    expect(tree.classList.contains('review-map-tree-hide')).toBeFalsy();
    expect(tree.classList.contains('review-map-tree-dim')).toBeFalsy();
  });
});

describe('plain-english strips', () => {
  test('buildStripRows anchors above the first mapped line, body = note else summary', () => {
    document.body.innerHTML = panelHTML + diffHTML;
    buildStripRows(stripsPageData(), document);
    const stripA = document.querySelector('#diff-aaaa tr.review-map-strip[data-bullet-index="0"]')!;
    expect((stripA.nextElementSibling as HTMLElement).id).toBe('a-add-3'); // first row inside 3-4
    expect(stripA.querySelector('.review-map-strip-title')!.textContent).toBe('Add the feature');
    expect(stripA.querySelector('.review-map-strip-text')!.textContent).toBe('Change-level text A.'); // no note -> summary
    expect(stripA.querySelector('.review-map-strip-no')!.textContent).toBe('1');
    expect(stripA.querySelector('.review-map-strip-info')!.getAttribute('title')).toBe('Change-level text A.');
    expect(stripA.querySelector('.review-map-strip-more')!.classList.contains('tw-hidden')).toBeFalsy(); // has details
    const stripB = document.querySelector('#diff-bbbb tr.review-map-strip[data-bullet-index="1"]')!;
    expect((stripB.nextElementSibling as HTMLElement).id).toBe('b-add-1');
    expect(stripB.querySelector('.review-map-strip-text')!.textContent).toBe('File note B.'); // note wins
  });

  test('the bullet filter keeps only the selected change strip visible', () => {
    document.body.innerHTML = panelHTML + diffHTML;
    buildStripRows(stripsPageData(), document);
    applyBulletFilter(stripsPageData().bullets[0], document);
    // same box now carries both strips: only the selected change's strip stays
    expect((document.querySelector('#diff-aaaa [data-bullet-index="0"]') as HTMLElement).classList.contains('tw-hidden')).toBeFalsy();
    expect((document.querySelector('#diff-aaaa [data-bullet-index="1"]') as HTMLElement).classList.contains('tw-hidden')).toBeTruthy();
    expect((document.querySelector('#diff-bbbb') as HTMLElement).classList.contains('tw-hidden')).toBeTruthy();
    clearBulletFilter(document);
    expect((document.querySelector('#diff-bbbb [data-bullet-index="1"]') as HTMLElement).classList.contains('tw-hidden')).toBeFalsy();
  });

  test('More toggles the details block and Close collapses an open strip', () => {
    document.body.innerHTML = panelHTML + diffHTML;
    buildStripRows(stripsPageData(), document);
    const stripA = document.querySelector('#diff-aaaa tr.review-map-strip[data-bullet-index="0"]') as HTMLElement;
    (stripA.querySelector('.review-map-strip-more') as HTMLElement).click();
    expect(stripA.querySelector('.review-map-strip-details')!.classList.contains('tw-hidden')).toBeFalsy();
    (stripA.querySelector('.review-map-strip-close') as HTMLElement).click();
    expect(stripA.classList.contains('review-map-strip-open')).toBeFalsy();
    stripA.click(); // anywhere else on the strip toggles it open again
    expect(stripA.classList.contains('review-map-strip-open')).toBeTruthy();
  });
});

describe('?map= deep link', () => {
  test('a map URL param applies that change on load and wins over sessionStorage', () => {
    document.body.innerHTML = panelHTML + diffHTML;
    sessionStorage.setItem('review-map-filter', '0');
    window.history.replaceState(null, '', '/owner/repo/pulls/1/files?map=1');
    window.config.pageData.reviewMap = pageData;
    initReviewMapPanel();
    expect(document.querySelector('.review-map-bullet.review-map-active')!.getAttribute('data-bullet-index')).toBe('1');
    expect(window.location.search).toContain('map=1');
    (document.querySelector('#review-map-show-all') as HTMLElement).click();
    expect(window.location.search).not.toContain('map=');
    window.history.replaceState(null, '', '/');
  });
});

describe('discussion flags', () => {
  test('nextUnapprovedBulletIndex never walks onto a flagged change', () => {
    const approval = [
      {approved: 2, total: 2},
      {approved: 0, total: 3}, // flagged
      {approved: 0, total: 4},
    ];
    expect(nextUnapprovedBulletIndex(approval, 0, new Set([1]))).toBe(2);
    expect(nextUnapprovedBulletIndex(approval, 2, new Set([1]))).toBe(-1);
  });

  test('flagging posts the flag key, clears approvals and marks the card', () => {
    document.body.innerHTML = panelHTML + diffHTML;
    window.config.pageData.reviewMap = pageData;
    initReviewMapPanel();
    const card = document.querySelector('[data-bullet-index="0"]') as HTMLElement;
    const flagBtn = card.querySelector('.review-map-flag-btn') as HTMLElement;
    (card.querySelector('.review-map-ref-check') as HTMLInputElement).checked = true;
    flagBtn.click();
    expect(flagBtn.classList.contains('review-map-flagged')).toBeTruthy();
    expect(card.classList.contains('review-map-is-flagged')).toBeTruthy();
    expect((card.querySelector('.review-map-ref-check') as HTMLInputElement).checked).toBeFalsy();
    expect(POST).toHaveBeenCalledWith('/owner/repo/pulls/1/review-map/progress', {data: {hash: 'hash-a', flag: true, reviewed: true}});
    expect(POST).toHaveBeenCalledWith('/owner/repo/pulls/1/review-map/progress', {data: {hash: 'hash-a', refIndex: 0, reviewed: false}});
  });
});

describe('card state spine', () => {
  test('approving every ref adds the approved class to the card', () => {
    document.body.innerHTML = panelHTML + diffHTML;
    window.config.pageData.reviewMap = pageData;
    initReviewMapPanel();
    const card = document.querySelector('[data-bullet-index="0"]') as HTMLElement;
    const check = card.querySelector('.review-map-ref-check') as HTMLInputElement;
    check.checked = true;
    check.dispatchEvent(new Event('change', {bubbles: true}));
    expect(card.classList.contains('review-map-approved')).toBeTruthy();
    check.checked = false;
    check.dispatchEvent(new Event('change', {bubbles: true}));
    expect(card.classList.contains('review-map-approved')).toBeFalsy();
  });
});

describe('tab card controls do not navigate', () => {
  test('clicking the approve chip on a non-files page stays put', () => {
    document.body.innerHTML = panelHTML;
    const spyAssign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
    window.config.pageData.reviewMap = pageData;
    initReviewMapPanel();
    (document.querySelector('.review-map-approve-chip') as HTMLElement).click();
    (document.querySelector('.review-map-flag-btn') as HTMLElement).click();
    expect(spyAssign).not.toHaveBeenCalled();
    expect(sessionStorage.getItem('review-map-filter')).toBeNull();
    spyAssign.mockRestore();
  });
});

describe('tab page wiring', () => {
  test('chip and flag handlers are live on non-files pages (no navigation)', () => {
    document.body.innerHTML = panelHTML;
    const spyAssign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
    window.config.pageData.reviewMap = pageData;
    initReviewMapPanel();
    const card = document.querySelector('[data-bullet-index="0"]') as HTMLElement;
    (card.querySelector('.review-map-ref-check') as HTMLInputElement).checked = true;
    (card.querySelector('.review-map-ref-check') as HTMLInputElement).dispatchEvent(new Event('change', {bubbles: true}));
    expect(POST).toHaveBeenCalledWith('/owner/repo/pulls/1/review-map/progress', {data: {hash: 'hash-a', refIndex: 0, reviewed: true}});
    expect(card.classList.contains('review-map-approved')).toBeTruthy();
    expect(spyAssign).not.toHaveBeenCalled();
    spyAssign.mockRestore();
  });
});
