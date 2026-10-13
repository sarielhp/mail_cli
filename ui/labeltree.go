package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/label"

	"github.com/fatih/color"
)

// FormatLabelTree formats and writes the hierarchical label tree to the given io.Writer.
func FormatLabelTree(w io.Writer, headerTitle string, items []label.Item, hideZero bool) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint("======================================================================"))
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint(headerTitle))
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint("======================================================================"))

	if len(items) == 0 {
		fmt.Fprintln(w, "  No labels/folders found.")
		return
	}

	tree := label.BuildLabelTree(items)

	var printNode func([]*label.TreeNode, string, int, bool)
	printNode = func(nodes []*label.TreeNode, prefix string, depth int, hideZero bool) {
		var visible []*label.TreeNode
		for _, node := range nodes {
			if hideZero && node.MessagesTotal == 0 {
				continue
			}
			visible = append(visible, node)
		}

		colors := []*color.Color{
			app.ColorBoldCyan,
			app.ColorBoldGreen,
			app.ColorBoldYellow,
			app.ColorBoldPurple,
			app.ColorWhite,
		}
		depthColor := colors[depth%len(colors)]

		for i, node := range visible {
			isLast := i == len(visible)-1
			connector := "├── "
			if isLast {
				connector = "└── "
			}

			nameStyled := depthColor.Sprint(node.Name)

			if node.IsLabel {
				if node.MessagesUnread > 0 {
					fmt.Fprintf(w, "%s%s%s (%d/%d)\n", prefix, connector, nameStyled, node.MessagesUnread, node.MessagesTotal)
				} else {
					fmt.Fprintf(w, "%s%s%s (%d)\n", prefix, connector, nameStyled, node.MessagesTotal)
				}
			} else {
				fmt.Fprintf(w, "%s%s%s [Folder]\n", prefix, connector, nameStyled)
			}

			if len(node.Children) > 0 {
				nextPrefix := prefix + "│   "
				if isLast {
					nextPrefix = prefix + "    "
				}
				printNode(node.Children, nextPrefix, depth+1, hideZero)
			}
		}
	}

	printNode(tree, "", 0, hideZero)
}

// FormatLabelListing writes a complete label listing: the server's own folders
// first, as a flat list, then the user's hierarchy as a tree.
//
// Each backend used to print its own listing, so the three had drifted into
// three layouts and JMAP's ran to fifty lines that re-derived what its
// GetLabelItems already returned. Splitting on IsSystem keeps Gmail's two-part
// display without the other backends having to know about it: a backend that
// marks nothing as a system folder simply gets the tree.
func FormatLabelListing(w io.Writer, backend string, items []label.Item, hideZero bool) {
	var system, user []label.Item
	for _, it := range items {
		if it.IsSystem {
			system = append(system, it)
			continue
		}
		user = append(user, it)
	}

	name := strings.ToUpper(backend)
	if len(system) > 0 {
		fmt.Fprintln(w)
		app.ColorBoldCyan.Fprintln(w, strings.Repeat("=", 70))
		app.ColorBoldCyan.Fprintf(w, "%s\n", centreTitle(name+" SYSTEM FOLDERS"))
		app.ColorBoldCyan.Fprintln(w, strings.Repeat("=", 70))
		for _, it := range system {
			if hideZero && it.MessagesTotal == 0 {
				continue
			}
			fmt.Fprintf(w, "  \u2022 %-25s  [Total: %-4d  Unread: %-4d]\n",
				it.Name, it.MessagesTotal, it.MessagesUnread)
		}
	}

	FormatLabelTree(w, centreTitle(name+" FOLDERS"), user, hideZero)
}

// centreTitle pads a title into the 70-column rule the listings use.
func centreTitle(title string) string {
	const width = 70
	if len(title) >= width {
		return title
	}
	left := (width - len(title)) / 2
	return strings.Repeat(" ", left) + title + strings.Repeat(" ", width-len(title)-left)
}
