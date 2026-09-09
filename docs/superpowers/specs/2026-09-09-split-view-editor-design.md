# Split-View Editor — Design

## 1. Overview

The editor area currently holds one editor group: a tab strip (`app.Model.tabs`) plus one active tab's editor. This spec adds a second, optional editor group side by side with the first — a fixed left/right split, not arbitrary nested splitting — so the user can view and edit two files at once.

Confirmed decisions (from user):
- **Split semantics**: each pane is a genuinely separate editor group with its own tab strip. Splitting *moves* a tab out of its current pane's strip into the other pane's strip — it does not duplicate/mirror the buffer.
- **Split depth**: exactly two panes maximum (left, right). No recursive/nested splitting.

Everything else below is this session's own judgment call, made with the user's explicit authorization to decide and proceed.

## 2. Global Constraints

- No new third-party dependencies.
- Single-pane (unsplit) behavior must be pixel- and behavior-identical to today — this spec adds a second pane, it does not change how the existing one works when alone.
- All existing tests must keep passing; call sites broken by the `tabs`/`activeTab` → `panes`/`activePane` refactor are updated, not deleted.
- A pane can never be left with zero tabs while a split exists — an operation that would empty one collapses the layout back to a single pane instead.
- Reuse the existing dropdown-menu pattern (`renderDropdown`/`clickMenuLabel` in `menu.go`) for the tab's right-click menu, not the file tree's spliced-row context-menu pattern — tabs are a horizontal strip like the menu bar, not a vertical row list.

## 3. Data model

### 3.1 `app.editorPane`

```go
// editorPane is one editor group: its own open tabs and which one is
// active. len(app.Model.panes) == 1 means a single unsplit editor area;
// == 2 means the editor area is split left/right, panes[0] on the left.
type editorPane struct {
	tabs      []tab
	activeTab int // -1 when this pane has no tabs open
}
```

### 3.2 `app.Model` changes

Remove `tabs []tab` and `activeTab int`. Add:

```go
panes      []editorPane // len 1 (unsplit) or 2 (split); never 0
activePane int          // which pane index keyboard/mouse edits target
splitCol   int          // on-screen column of the boundary between
                         // panes[0]/panes[1] — mutable via drag, like
                         // treeWidth/terminalHeight. Meaningful only
                         // when len(panes) == 2.
```

`New()` initializes `panes: []editorPane{{activeTab: -1}}`, `activePane: 0`.

### 3.3 Generalized helpers

Every existing single-pane helper becomes pane-aware. `activeEditor()`/`setActiveEditor()` keep their exact signatures (operating on `m.panes[m.activePane]`) so the ~20 call sites elsewhere in `app` that already use them are untouched:

```go
func (m Model) activeEditor() editor.Model {
	p := m.panes[m.activePane]
	if p.activeTab < 0 || p.activeTab >= len(p.tabs) {
		return editor.Model{}
	}
	return p.tabs[p.activeTab].editor
}

func (m Model) setActiveEditor(e editor.Model) Model {
	p := &m.panes[m.activePane]
	if p.activeTab >= 0 && p.activeTab < len(p.tabs) {
		p.tabs[p.activeTab].editor = e
	}
	return m
}
```

`openOrSwitch` searches **all** panes for an already-open path (not just the active one) before creating a new tab — the existing "opening a file switches to its tab instead of duplicating" guarantee now holds pane-globally, so the same file is never open as two independently-diverging tabs in two panes at once:

```go
func (m Model) openOrSwitch(path string, editorW, editorH int) (Model, error) {
	for pi := range m.panes {
		for i, t := range m.panes[pi].tabs {
			if t.path == path {
				m.activePane = pi
				m.panes[pi].activeTab = i
				return m, nil
			}
		}
	}
	editorModel, err := editor.New().LoadFile(path)
	if err != nil {
		return m, err
	}
	editorModel = editorModel.SetSize(editorW, editorH)
	p := &m.panes[m.activePane]
	p.tabs = append(p.tabs, tab{path: path, editor: editorModel})
	p.activeTab = len(p.tabs) - 1
	return m, nil
}
```

`closeTab`/`removeTab` (in `confirm.go`) gain a `pane int` parameter alongside their existing `index int`, otherwise keeping the same dirty-check-then-confirm-or-remove shape. `removeTab`'s post-removal bookkeeping gains one more case, handling **either** side emptying while a split exists (`len(m.panes) == 2`) — this is the one place that rule lives; nothing else needs its own copy of it:

- Pane 1 emptied: drop `panes[1]` entirely, `activePane = 0`.
- Pane 0 emptied *and pane 1 still has tabs*: `panes[0] = panes[1]`, then drop the now-duplicated `panes[1]`, `activePane = 0`. (Pane 0 is where View() always renders "the" single pane in unsplit mode, so the survivor moves into that slot rather than leaving pane 1 as a lone populated pane at index 1.)
- Both would be empty (only reachable if the model somehow already violated "never empty both" — defensive, not a normal path): fall back to `panes = []editorPane{{activeTab: -1}}`, `activePane = 0`.

This single rule is what makes `moveTabToOtherPane` (4.1) self-consistent even in the degenerate case of moving a pane's *only* tab: removing it from the source empties that pane, the rule above immediately collapses back to one populated pane — correct, since there was never a second tab to justify showing two panes.

### 3.4 `newTabEditorSize`

Currently returns one `(width, height)` pair for "the" editor. It gains a `pane int` parameter: height is unaffected by the split (only the terminal/tab-bar stack vertically), but width must reflect that pane's actual on-screen share once split (see 4.4's geometry) rather than always `m.width - m.treeWidth - borderSize`.

## 4. Behavior

### 4.1 Splitting and moving a tab

New method, replacing nothing (this is new behavior):

```go
// moveTabToOtherPane moves the tab at (pane, index) into the other pane
// (0 <-> 1), creating panes[1] first if it doesn't exist yet — this is
// what "split" means here: there is no separate "create an empty split"
// operation, only "move a tab into the other side, creating that side if
// needed." If this empties the source pane, removeTab's collapse rule
// (3.3) takes over.
func (m Model) moveTabToOtherPane(pane, index int) Model {
	if len(m.panes) == 1 {
		m.panes = append(m.panes, editorPane{activeTab: -1})
		m.splitCol = m.defaultSplitCol()
	}
	target := 1 - pane
	moved := m.panes[pane].tabs[index]
	// removeTabAt does the same removal + activeTab reassignment +
	// empty-pane collapse (3.3) that removeTab already does for a plain
	// close — factored out so both callers share one implementation
	// instead of duplicating that branch-by-branch logic.
	m = m.removeTabAt(pane, index)
	// If removeTabAt just collapsed panes[1] away (source was pane 0's
	// only tab, target was the freshly-created empty pane 1), "target"
	// as an index is now stale — recompute which pane index the moved
	// tab actually belongs in.
	if len(m.panes) == 1 {
		target = 0
	}
	m.panes[target].tabs = append(m.panes[target].tabs, moved)
	m.panes[target].activeTab = len(m.panes[target].tabs) - 1
	m.activePane = target
	m.focus = focusEditor
	return m
}
```

(`removeTabAt` is `removeTab`'s existing removal/reassignment/collapse body, factored out under a name both `removeTab` and `moveTabToOtherPane` call, rather than duplicating it.)

`defaultSplitCol()` returns the horizontal midpoint of the editor area (`m.treeWidth + (m.width-m.treeWidth)/2`), clamped the same way a drag would be (4.5).

### 4.2 Right-click tab menu

New file `internal/app/tabmenu.go`. New `Model` field:

```go
tabMenu *tabContextMenu // nil when no tab menu is open

type tabContextMenu struct {
	pane  int
	index int
}
```

One dropdown, one item for now: "Split + Move Right" when `tabMenu.pane == 0`, "Move Left" when `tabMenu.pane == 1` — both routed to the same `moveTabToOtherPane(tabMenu.pane, tabMenu.index)`. Rendered and hit-tested with the exact same pattern as the File/Edit dropdown (`renderDropdown`/`dropdownStyle`/`dropdownBorderSize` in `menu.go`, `clickMenuLabel`/the dropdown branch of `handleClick` in `model.go`), anchored at the right-clicked tab's screen column instead of a menu label's. This keeps a single dropdown "look" in the app and leaves room to add more tab-menu items later without a redesign.

`handleRightClick` (currently tree-only) gains a case: a right click inside a tab-bar rect opens `tabMenu` for the tab under it (via the pane's existing `tabAt` hit-test), closing whichever OTHER dropdown/menu state might be open (mirroring how opening the File dropdown already closes an unrelated one).

Dismissal: a right-clicked-open tab menu is a dialog-shaped piece of transient UI, so it's dismissed the same way the modal dialogs already are (Esc, or **any left click**, per PR #4's generic mouse-dismiss — extend that same rewrite-to-Esc branch to also fire when `m.tabMenu != nil`, not only `m.activeDialog != dialogNone`), plus selecting its one item.

### 4.3 Closing a tab in a split

`closeTab(pane, index)` is otherwise unchanged from today's single-pane version (dirty check → confirm dialog or immediate `removeTab`). Closing pane 0's only tab while pane 1 still has tabs open is exactly the "pane 0 emptied, pane 1 survives" case `removeTab`'s collapse rule (3.3) already covers — no separate handling needed here.

### 4.4 Rendering (`View()`)

Unsplit (`len(m.panes) == 1`): identical to today's rendering, verbatim.

Split (`len(m.panes) == 2`): the column currently occupied by one tab-bar-over-editor stack becomes two side-by-side stacks, sharing a border the same way the tree and editor panes already visually abut (no extra gap). Terminal is unaffected — it stays a single full-width block below both, spanning `[m.treeWidth, m.width)` exactly as it does today.

```
┌──────────────────────────────────────────────────────┐
│ File  Edit  Commands  About                           │  menu bar
├───────────┬─────────────────────┬────────────────────┤
│           │ a.go  b.go×         │ c.go×               │  tab bar (per pane)
│  project  ├─────────────────────┼────────────────────┤
│  tree     │                     │                     │
│           │   panes[0]          │   panes[1]          │
│           │                     │                     │
│           ├─────────────────────┴────────────────────┤
│           │           terminal (unaffected)            │
├───────────┴──────────────────────────────────────────┤
│ status bar                                             │
└──────────────────────────────────────────────────────┘
```

Geometry: `pane0Width = m.splitCol - m.treeWidth`, `pane1Width = m.width - m.splitCol`. Each pane's tab bar renders at its own pane's width (mirroring how the single tab bar today renders at `m.width - m.treeWidth`); each pane's editor box border is colored `focusedBorderColor` only when `m.focus == focusEditor && m.activePane == <that pane>` — with a split, "the" editor being focused is no longer enough to know which box to highlight.

Both panes are guaranteed non-empty whenever a split exists (3.3, 4.3), so both always show a tab bar at the same height — the two editor boxes stay vertically aligned with each other and with the terminal below, with no per-pane height special-casing needed.

### 4.5 Resizing the split

New `resizeKind` value `resizeEditorSplit`. `beginResizeDrag` gains a case: when `len(m.panes) == 2`, a press exactly on the column between the two editor rects (mirroring the existing tree-border and editor/terminal-boundary checks) starts this drag. `applyResizeDrag` gains a matching case updating `m.splitCol` via a new `clampSplitCol`, following `clampTreeWidth`'s exact shape:

```go
// clampSplitCol keeps a candidate split boundary within
// [m.treeWidth+minEditorWidth, m.width-minEditorWidth] so dragging can
// never collapse either side below minEditorWidth. Reuses clampInt, same
// degenerate-window handling as clampTreeWidth/clampTerminalHeight.
func (m Model) clampSplitCol(col int) int {
	return clampInt(col, m.treeWidth+minEditorWidth, m.width-minEditorWidth)
}
```

### 4.6 Keyboard focus cycling

`focusNext`/`focusPrev` gain one more stop when a split exists: `tree → panes[0] → panes[1] (if split) → terminal → tree` (and the reverse for Shift+Tab). Concretely, `focusNext`'s `focusEditor` case becomes: if `m.focus == focusEditor && m.activePane == 0 && len(m.panes) == 2`, stay on `focusEditor` and set `m.activePane = 1`; otherwise fall through to `focusTerminal` as today. `focusPrev` mirrors this. `m.activePane` is left wherever it was on leaving editor focus, so Tab-ing back into the editor later resumes on the same side.

### 4.7 Mouse routing

`paneLayout()`'s signature changes from four named rects to a small struct, since the number of editor/tab-bar rects is no longer fixed at one:

```go
type editorPaneLayout struct {
	tabBar, editor rect
}

type layout struct {
	tree     rect
	panes    []editorPaneLayout // len matches m.panes
	terminal rect
}

func (m Model) paneLayout() layout
```

Every existing caller (`handlePaneClick`, `handleWheel`, `handleRightClick`, `beginResizeDrag`, `applyResizeDrag`) is updated to loop over `layout.panes` (indexed 0/1) instead of destructuring a fixed `tabBar, editorR` pair — e.g. `handlePaneClick` becomes: for each `pi, pl := range layout.panes`, check `pl.tabBar.contains` / `pl.editor.contains`, and on a hit, set `m.activePane = pi` before the existing tab-click / cursor-click logic (which already operates via `activeEditor()`/`setActiveEditor()` and is otherwise untouched).

## 5. Testing

- `openOrSwitch` finds an existing tab in the *other* pane and switches to it (no duplicate created).
- `moveTabToOtherPane` on an unsplit model: creates `panes[1]`, moves the tab, sets `activePane = 1`, `focus = focusEditor`.
- `moveTabToOtherPane` back (pane 1 → pane 0) when pane 1's moved tab was its only one: collapses to a single pane.
- `moveTabToOtherPane` into an already-split pane 1 that has other tabs: appends without disturbing pane 1's existing tabs.
- Closing pane 0's only tab while pane 1 still has tabs: pane 1 survives as the new (unsplit) pane 0.
- Right-click a tab opens the menu with the pane-appropriate label ("Split + Move Right" / "Move Left"); selecting it (or a left click elsewhere, or Esc) behaves as designed in 4.2.
- Dragging the new split boundary resizes `splitCol`, clamped at both ends (mirroring the existing tree-width clamp tests).
- Tab/Shift+Tab cycles through both panes when split, skips the second when not.
- Clicking in pane 1's tab bar / editor area routes correctly (`activePane` switches, tab selection / cursor placement land in the right pane) — mirrors the existing single-pane click-routing tests, doubled.
- Full existing single-pane test suite passes unchanged (behavioral parity check for `len(m.panes) == 1`).
