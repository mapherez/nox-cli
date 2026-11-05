// Package navigation builds a local, read-only menu from verified Gateway data.
package navigation

import (
	"cmp"
	"sort"
	"strings"

	"github.com/mapherez/nox-cli/internal/gateway"
	noxmcp "github.com/mapherez/nox-mcp/go"
)

type Action struct {
	Alias, Fingerprint, Description, Problem string
	Definition                               noxmcp.JSON
}

type Node struct {
	ID, Label   string
	Order       float64
	Children    []*Node
	Action      *Action
	Diagnostics []string
}

type Tree struct {
	Root        *Node
	Diagnostics []string
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func text(v any) string           { s, _ := v.(string); return s }

// Build never infers product identity from an alias or invents a missing CLI path.
// IDs and path segments remain canonical; labels are presentation only.
func Build(c gateway.Catalog) Tree {
	tree := Tree{Root: &Node{Label: "Apps"}}
	products := map[string]*Node{}
	if rows, ok := c.Presentation["products"].([]any); ok {
		for _, value := range rows {
			row := object(value)
			id := text(row["productId"])
			if id == "" {
				continue
			}
			if products[id] != nil {
				tree.Diagnostics = append(tree.Diagnostics, "Duplicate product presentation: "+id)
				continue
			}
			label := text(row["displayName"])
			if strings.TrimSpace(label) == "" {
				label = id
			}
			products[id] = &Node{ID: id, Label: label}
		}
	}
	names := make([]string, 0, len(c.Definitions))
	for name := range c.Definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, alias := range names {
		definition := c.Definitions[alias]
		tool := object(definition["tool"])
		meta := object(tool["_meta"])
		provenance := object(meta["nox/gateway"])
		id := text(provenance["productId"])
		if id == "" || provenance["version"] != "1.0" {
			tree.Diagnostics = append(tree.Diagnostics, "Missing Gateway product identity: "+alias)
			continue
		}
		product := products[id]
		if product == nil {
			product = &Node{ID: id, Label: id}
			products[id] = product
		}
		path := strings.Fields(text(meta["cli"]))
		if len(path) == 0 {
			product.Diagnostics = append(product.Diagnostics, "No CLI path: "+alias)
			continue
		}
		action := &Action{Alias: alias, Fingerprint: text(definition["fingerprint"]), Description: text(tool["description"]), Definition: definition}
		var descriptor noxmcp.JSON
		if raw, exists := definition["cli"]; exists {
			var err error
			descriptor, err = noxmcp.InterpretCLI(object(raw))
			if err != nil {
				action.Problem = "Unsupported or invalid CLI descriptor."
			}
		}
		if definition["availability"] == "unavailable" {
			action.Problem = "The Gateway reports this tool as unavailable."
		}
		nav := object(descriptor["navigation"])
		order, _ := nav["order"].(float64)
		parent := product
		for i, segment := range path {
			var child *Node
			for _, n := range parent.Children {
				if n.ID == segment {
					child = n
					break
				}
			}
			if child == nil {
				child = &Node{ID: segment, Label: segment, Order: order}
				parent.Children = append(parent.Children, child)
			}
			child.Order = min(child.Order, order)
			if i == len(path)-2 && text(nav["group"]) != "" {
				child.Label = text(nav["group"])
			}
			parent = child
		}
		if label := text(nav["label"]); label != "" {
			parent.Label = label
		} else if title := text(tool["title"]); title != "" {
			parent.Label = title
		}
		if parent.Action != nil {
			parent.Action.Problem = "Multiple tools publish this CLI path. Tool selection is ambiguous."
			parent.Diagnostics = append(parent.Diagnostics, "Conflicting tool: "+alias)
		} else {
			parent.Action = action
		}
	}
	for _, product := range products {
		tree.Root.Children = append(tree.Root.Children, product)
	}
	sortNodes(tree.Root)
	return tree
}

func sortNodes(root *Node) {
	pending := []*Node{root}
	for len(pending) > 0 {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		sort.Slice(node.Children, func(i, j int) bool {
			a, b := node.Children[i], node.Children[j]
			if a.Order != b.Order {
				return a.Order < b.Order
			}
			if n := cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)); n != 0 {
				return n < 0
			}
			return a.ID < b.ID
		})
		pending = append(pending, node.Children...)
	}
}

// Find returns the nearest surviving ancestor after an explicit catalog refresh.
func (t Tree) Find(route []string) (*Node, []string) {
	node := t.Root
	valid := []string{}
	for _, id := range route {
		var next *Node
		for _, child := range node.Children {
			if child.ID == id {
				next = child
				break
			}
		}
		if next == nil {
			break
		}
		node = next
		valid = append(valid, id)
	}
	return node, valid
}
