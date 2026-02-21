package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// STYLES
// ═══════════════════════════════════════════════════════════════════════════════

var (
	// Colors
	purple     = lipgloss.Color("#BD93F9")
	pink       = lipgloss.Color("#FF79C6")
	cyan       = lipgloss.Color("#8BE9FD")
	green      = lipgloss.Color("#50FA7B")
	orange     = lipgloss.Color("#FFB86C")
	red        = lipgloss.Color("#FF5555")
	comment    = lipgloss.Color("#6272A4")
	foreground = lipgloss.Color("#F8F8F2")
	background = lipgloss.Color("#282A36")
	selection  = lipgloss.Color("#44475A")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(foreground).
			Background(purple).
			Padding(0, 2)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(comment).
			Italic(true)

	// Tree connectors
	treeLineStyle = lipgloss.NewStyle().Foreground(comment)

	// Item styles
	normalStyle = lipgloss.NewStyle().Foreground(foreground)

	selectedBgStyle = lipgloss.NewStyle().
			Background(selection).
			Foreground(foreground)

	cursorStyle = lipgloss.NewStyle().Foreground(pink).Bold(true)

	checkboxChecked   = lipgloss.NewStyle().Foreground(green).Bold(true)
	checkboxUnchecked = lipgloss.NewStyle().Foreground(comment)
	checkboxPartial   = lipgloss.NewStyle().Foreground(orange).Bold(true)

	sizeSmall  = lipgloss.NewStyle().Foreground(green)
	sizeMedium = lipgloss.NewStyle().Foreground(orange)
	sizeLarge  = lipgloss.NewStyle().Foreground(red).Bold(true)

	deletedStyle = lipgloss.NewStyle().Foreground(red).Strikethrough(true)

	pathDisplayStyle = lipgloss.NewStyle().
				Foreground(orange).
				Italic(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(comment).
				Padding(0, 1)

	statsStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(comment)

	spinnerStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
)

// ═══════════════════════════════════════════════════════════════════════════════
// DATA STRUCTURES
// ═══════════════════════════════════════════════════════════════════════════════

type TreeNode struct {
	Path       string
	Name       string
	Size       int64
	Selected   bool
	Deleted    bool
	Expanded   bool
	Children   []*TreeNode
	Parent     *TreeNode
	IsLast     bool // Is this the last child of its parent?
}

type model struct {
	root         *TreeNode        // Virtual root containing all top-level items
	flatList     []*TreeNode      // Flattened list for navigation
	cursor       int
	scanning     bool
	width        int
	height       int
	quitting     bool
	scrollOffset int
	cwd          string
}

type foundMsg struct {
	path string
	size int64
}

type scanDoneMsg struct{}
type tickMsg time.Time

// ═══════════════════════════════════════════════════════════════════════════════
// TREE OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════════

func newTreeNode(path, name string, size int64) *TreeNode {
	return &TreeNode{
		Path:     path,
		Name:     name,
		Size:     size,
		Expanded: true,
		Children: make([]*TreeNode, 0),
	}
}

// Insert a path into the tree, creating intermediate nodes as needed
func (m *model) insertPath(fullPath string, size int64) {
	relPath, _ := filepath.Rel(m.cwd, fullPath)
	parts := strings.Split(relPath, string(filepath.Separator))

	current := m.root
	currentPath := m.cwd

	for i, part := range parts {
		currentPath = filepath.Join(currentPath, part)
		isNodeModules := part == "node_modules"
		isLast := i == len(parts)-1

		// Find or create child
		var child *TreeNode
		for _, c := range current.Children {
			if c.Name == part {
				child = c
				break
			}
		}

		if child == nil {
			nodeSize := int64(0)
			if isNodeModules && isLast {
				nodeSize = size
			}
			child = newTreeNode(currentPath, part, nodeSize)
			child.Parent = current
			current.Children = append(current.Children, child)
			// Sort children by total size (including all descendants)
			sort.Slice(current.Children, func(i, j int) bool {
				sizeI := current.Children[i].getTotalSize()
				sizeJ := current.Children[j].getTotalSize()
				if sizeI != sizeJ {
					return sizeI > sizeJ
				}
				return current.Children[i].Name < current.Children[j].Name
			})
		} else if isNodeModules && isLast {
			child.Size = size
		}

		current = child
	}

	m.rebuildFlatList()
}

// Rebuild flat list from tree for navigation
func (m *model) rebuildFlatList() {
	m.flatList = make([]*TreeNode, 0)
	m.flattenNode(m.root, 0)
	// Update IsLast flags
	for _, node := range m.root.Children {
		m.updateIsLastFlags(node)
	}
}

func (m *model) updateIsLastFlags(node *TreeNode) {
	for i, child := range node.Children {
		child.IsLast = i == len(node.Children)-1
		m.updateIsLastFlags(child)
	}
}

func (m *model) flattenNode(node *TreeNode, depth int) {
	// Don't add root to flat list
	if node != m.root {
		m.flatList = append(m.flatList, node)
	}

	if node.Expanded {
		for _, child := range node.Children {
			m.flattenNode(child, depth+1)
		}
	}
}

func (node *TreeNode) getDepth() int {
	depth := 0
	p := node.Parent
	for p != nil && p.Parent != nil { // Don't count virtual root
		depth++
		p = p.Parent
	}
	return depth
}

// Toggle selection and propagate to children
func (node *TreeNode) toggleSelect() {
	node.Selected = !node.Selected
	node.propagateSelectionDown(node.Selected)
	node.updateParentSelection()
}

func (node *TreeNode) propagateSelectionDown(selected bool) {
	node.Selected = selected
	for _, child := range node.Children {
		if !child.Deleted {
			child.propagateSelectionDown(selected)
		}
	}
}

func (node *TreeNode) updateParentSelection() {
	if node.Parent == nil || node.Parent.Parent == nil {
		return
	}
	parent := node.Parent
	allSelected := true
	anySelected := false
	for _, child := range parent.Children {
		if !child.Deleted {
			if child.Selected {
				anySelected = true
			} else {
				allSelected = false
			}
		}
	}
	parent.Selected = allSelected && anySelected
	parent.updateParentSelection()
}

// Check if node has some but not all children selected
func (node *TreeNode) hasPartialSelection() bool {
	if len(node.Children) == 0 {
		return false
	}
	selectedCount := 0
	totalCount := 0
	for _, child := range node.Children {
		if !child.Deleted {
			totalCount++
			if child.Selected || child.hasPartialSelection() {
				selectedCount++
			}
		}
	}
	return selectedCount > 0 && selectedCount < totalCount
}

// Get total size including children
func (node *TreeNode) getTotalSize() int64 {
	total := node.Size
	for _, child := range node.Children {
		total += child.getTotalSize()
	}
	return total
}

// ═══════════════════════════════════════════════════════════════════════════════
// BUBBLETEA MODEL
// ═══════════════════════════════════════════════════════════════════════════════

func initialModel() model {
	cwd, _ := os.Getwd()
	root := newTreeNode(cwd, "", 0)
	return model{
		root:     root,
		flatList: make([]*TreeNode, 0),
		scanning: true,
		cwd:      cwd,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.scanning {
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.scrollOffset = 0
			}

		case "down", "j":
			if m.cursor < len(m.flatList)-1 {
				m.cursor++
				m.scrollOffset = 0
			}

		case " ":
			if m.cursor >= 0 && m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]
				if !node.Deleted {
					node.toggleSelect()
				}
			}

		case "a":
			// Toggle all
			allSelected := true
			for _, node := range m.flatList {
				if !node.Selected && !node.Deleted {
					allSelected = false
					break
				}
			}
			for _, node := range m.flatList {
				if !node.Deleted {
					node.Selected = !allSelected
				}
			}

		case "e", "tab":
			// Toggle expand/collapse
			if m.cursor >= 0 && m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]
				if len(node.Children) > 0 {
					node.Expanded = !node.Expanded
					m.rebuildFlatList()
					// Keep cursor in bounds
					if m.cursor >= len(m.flatList) {
						m.cursor = len(m.flatList) - 1
					}
				}
			}

		case "enter":
			return m, deleteSelected(m.flatList)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case foundMsg:
		m.insertPath(msg.path, msg.size)

	case scanDoneMsg:
		m.scanning = false
		m.rebuildFlatList()

	case deletedMsg:
		// Refresh

	case tickMsg:
		m.scrollOffset++
		return m, tickCmd()
	}

	return m, nil
}

type deletedMsg struct{}

func deleteSelected(nodes []*TreeNode) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		for _, node := range nodes {
			if node.Selected && !node.Deleted && node.Size > 0 {
				wg.Add(1)
				go func(n *TreeNode) {
					defer wg.Done()
					os.RemoveAll(n.Path)
					n.Deleted = true
					n.Selected = false
				}(node)
			}
		}
		wg.Wait()
		return deletedMsg{}
	}
}

func calculateDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// ═══════════════════════════════════════════════════════════════════════════════
// VIEW
// ═══════════════════════════════════════════════════════════════════════════════

func (m model) View() string {
	if m.scanning {
		frames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
		idx := int(time.Now().UnixNano()/int64(time.Millisecond)/80) % len(frames)

		return fmt.Sprintf(`
  %s %s

  %s

  Found %d items so far...

  Press q to quit
`,
			spinnerStyle.Render(frames[idx]),
			spinnerStyle.Render("Scanning for node_modules..."),
			subtitleStyle.Render(m.cwd),
			len(m.flatList))
	}

	if len(m.flatList) == 0 {
		return `
   No node_modules folders found!

  Your directory is clean.

  Press q to quit
`
	}

	var s strings.Builder

	// Title
	s.WriteString("\n")
	s.WriteString(titleStyle.Render(" Node Modules Cleaner "))
	s.WriteString("\n")
	s.WriteString(subtitleStyle.Render("  " + m.cwd))
	s.WriteString("\n\n")

	// Calculate visible window
	windowHeight := m.height - 14
	if windowHeight < 5 {
		windowHeight = 5
	}

	top := 0
	if m.cursor > windowHeight/2 {
		top = m.cursor - windowHeight/2
	}
	bottom := top + windowHeight
	if bottom > len(m.flatList) {
		bottom = len(m.flatList)
		top = max(0, bottom-windowHeight)
	}

	// Render tree
	for i := top; i < bottom; i++ {
		node := m.flatList[i]
		s.WriteString(m.renderNode(node, i))
		s.WriteString("\n")
	}

	// Full path display
	s.WriteString("\n")
	if m.cursor >= 0 && m.cursor < len(m.flatList) {
		fullPath := m.flatList[m.cursor].Path
		maxWidth := m.width - 10
		if maxWidth < 30 {
			maxWidth = 30
		}
		if maxWidth > 80 {
			maxWidth = 80
		}

		displayPath := fullPath
		if len(fullPath) > maxWidth {
			// Scrolling
			cycle := len(fullPath) - maxWidth + 20
			pos := m.scrollOffset % cycle
			if pos < 10 {
				displayPath = fullPath[:maxWidth]
			} else if pos >= cycle-10 {
				displayPath = fullPath[len(fullPath)-maxWidth:]
			} else {
				start := pos - 10
				displayPath = fullPath[start : start+maxWidth]
			}
		}
		s.WriteString(pathDisplayStyle.Render("📁 " + displayPath))
	}
	s.WriteString("\n\n")

	// Stats
	selectedCount, selectedSize := m.getSelectionStats()
	s.WriteString(statsStyle.Render(fmt.Sprintf(
		"   Items: %d │ Selected: %d (%s) │ Total: %s",
		len(m.flatList),
		selectedCount,
		formatBytes(selectedSize),
		formatBytes(m.getTotalSize()),
	)))
	s.WriteString("\n")

	// Help
	s.WriteString(helpStyle.Render("    SPACE: select │ a: all │ TAB: expand/collapse │ ENTER: delete │ q: quit"))
	s.WriteString("\n")

	return s.String()
}

func (m model) renderNode(node *TreeNode, index int) string {
	depth := node.getDepth()
	isCursor := index == m.cursor

	// Build tree prefix
	var prefix strings.Builder

	// Traverse up to build the tree lines
	ancestors := make([]*TreeNode, 0)
	p := node.Parent
	for p != nil && p != m.root {
		ancestors = append([]*TreeNode{p}, ancestors...)
		p = p.Parent
	}

	// Draw vertical lines for ancestors
	for _, ancestor := range ancestors {
		if ancestor.IsLast {
			prefix.WriteString("   ")
		} else {
			prefix.WriteString(treeLineStyle.Render("│  "))
		}
	}

	// Draw connector for this node
	if depth > 0 {
		if node.IsLast {
			prefix.WriteString(treeLineStyle.Render("└──"))
		} else {
			prefix.WriteString(treeLineStyle.Render("├──"))
		}
	}

	// Expand/collapse indicator
	expandIndicator := " "
	if len(node.Children) > 0 {
		if node.Expanded {
			expandIndicator = treeLineStyle.Render("▼")
		} else {
			expandIndicator = treeLineStyle.Render("▶")
		}
	}

	// Cursor
	cursor := "  "
	if isCursor {
		cursor = cursorStyle.Render("❯ ")
	}

	// Checkbox
	checkbox := checkboxUnchecked.Render("○")
	if node.Selected {
		checkbox = checkboxChecked.Render("●")
	} else if node.hasPartialSelection() {
		checkbox = checkboxPartial.Render("◐")
	}
	if node.Deleted {
		checkbox = deletedStyle.Render("✗")
	}

	// Name and size
	name := node.Name
	if node.Deleted {
		name = deletedStyle.Render(name)
	} else if node.Size > 0 {
		name = normalStyle.Render(name)
	} else {
		name = subtitleStyle.Render(name) // Directory without size
	}

	// Size display - show total size including children
	sizeStr := ""
	totalSize := node.getTotalSize()
	if totalSize > 0 {
		sizeStr = formatSizeColored(totalSize)
		if len(node.Children) > 0 && node.Size == 0 {
			// This is a parent directory, show it's a sum
			sizeStr = subtitleStyle.Render("Σ ") + sizeStr
		}
	}

	// Assemble line
	line := fmt.Sprintf("%s%s%s %s %s %s",
		cursor,
		prefix.String(),
		expandIndicator,
		checkbox,
		name,
		sizeStr,
	)

	if isCursor {
		return selectedBgStyle.Render(line)
	}
	return line
}

func formatSizeColored(size int64) string {
	str := formatBytes(size)
	if size > 500*1024*1024 { // > 500MB
		return sizeLarge.Render(str)
	} else if size > 50*1024*1024 { // > 50MB
		return sizeMedium.Render(str)
	}
	return sizeSmall.Render(str)
}

func (m model) getSelectionStats() (int, int64) {
	count := 0
	var size int64
	for _, node := range m.flatList {
		if node.Selected && !node.Deleted && node.Size > 0 {
			count++
			size += node.Size
		}
	}
	return count, size
}

func (m model) getTotalSize() int64 {
	var total int64
	for _, node := range m.flatList {
		if !node.Deleted && node.Size > 0 {
			total += node.Size
		}
	}
	return total
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ═══════════════════════════════════════════════════════════════════════════════
// MAIN
// ═══════════════════════════════════════════════════════════════════════════════

func main() {
	p := tea.NewProgram(initialModel())

	go func() {
		cwd, _ := os.Getwd()
		filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && d.Name() == "node_modules" {
				size := calculateDirSize(path)
				p.Send(foundMsg{path: path, size: size})
				return filepath.SkipDir
			}
			return nil
		})
		p.Send(scanDoneMsg{})
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

