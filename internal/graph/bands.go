package graph

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// The viewer gives a band only to the layers holding band-level items: the top-level
// ones or, when the top level holds nothing but one folder box, that box's items.
// Layers never split a folder box, so a layer whose packages all sit inside a folder
// another layer owns would get no band, and the author would only find out in the
// browser, so the config is checked here. This is not layout: it only works out which
// items bands are made of, mirroring buildTree, flattenTree, toItem and bandBox in
// app/server/index.html. Keep the two in sync.

// folder is one directory of the folder tree the viewer draws as boxes.
type folder struct {
	path   string // in node ID form: "." for the root, "./a/b" below it
	pkg    bool   // a package lives in it
	parent *folder
	// kids is keyed by path, not name, as a flattened folder's child can share its
	// name with a sibling.
	kids map[string]*folder
}

// bandItem is an item bands are made of: a package or a folder box.
type bandItem struct {
	path string   // what the viewer looks its layer up by
	ids  []string // the packages inside it
	box  *folder  // the folder it draws as a box, or nil for a package
}

// folderTree builds the folder tree of the nodes and dissolves the flattened
// folders into their parents, deepest first, as the viewer does.
func folderTree(nodes []Node, flatten []string) *folder {
	root := &folder{path: ".", kids: map[string]*folder{}}
	byPath := map[string]*folder{".": root}
	for _, n := range nodes {
		if n.ID == "." {
			root.pkg = true
			continue
		}
		cur := root
		for seg := range strings.SplitSeq(strings.TrimPrefix(n.ID, "./"), "/") {
			p := cur.path + "/" + seg
			next := byPath[p]
			if next == nil {
				next = &folder{path: p, parent: cur, kids: map[string]*folder{}}
				byPath[p] = next
				cur.kids[p] = next
			}
			cur = next
		}
		cur.pkg = true
	}
	order := slices.Clone(flatten)
	slices.SortStableFunc(order, func(a, b string) int { return strings.Count(b, "/") - strings.Count(a, "/") })
	for _, p := range order {
		if f := byPath[p]; f != nil {
			f.dissolve()
		}
	}
	return root
}

// dissolve moves f's package and subfolders up into its parent. A folder without
// subfolders has nothing to dissolve (flattenWarnings reports it), and a dissolved
// one is detached, so a repeated entry does nothing.
func (f *folder) dissolve() {
	if f.parent == nil || len(f.kids) == 0 {
		return
	}
	p := f.parent
	delete(p.kids, f.path)
	if f.pkg {
		p.kids[f.path] = &folder{path: f.path, pkg: true, parent: p, kids: map[string]*folder{}}
	}
	for _, k := range f.kids {
		k.parent = p
		p.kids[k.path] = k
	}
	f.parent, f.kids = nil, nil
}

// ids returns the packages in f and below it.
func (f *folder) ids() []string {
	var out []string
	if f.pkg {
		out = append(out, f.path)
	}
	for _, k := range f.kids {
		out = append(out, k.ids()...)
	}
	return out
}

// bandItems returns the items bands are made of, and the folder holding them: the
// ones root holds or, while they are a single folder box, the ones inside it.
func bandItems(root *folder) ([]bandItem, *folder) {
	in := root
	items := itemsOf(root)
	for len(items) == 1 && items[0].box != nil {
		in = items[0].box
		items = itemsOf(in)
	}
	return items, in
}

// itemsOf returns the items f holds, sorted by path: its own package, whose path is
// f's ("." for the root), and its subfolders. A folder holding nothing but one
// subfolder is drawn as that subfolder (labelled "a/b"), so it takes its path.
func itemsOf(f *folder) []bandItem {
	var out []bandItem
	if f.pkg {
		out = append(out, bandItem{path: f.path, ids: []string{f.path}})
	}
	for _, k := range f.kids {
		for !k.pkg && len(k.kids) == 1 {
			for _, only := range k.kids {
				k = only
			}
		}
		it := bandItem{path: k.path, ids: k.ids()}
		if len(k.kids) > 0 {
			it.box = k
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// layerIndex looks up which layer an item belongs to, like layerOf in the viewer.
type layerIndex struct {
	byEntry map[string]int
	rest    int // the "*" layer, or len(layers) when there is none
}

// newLayerIndex indexes the layers' entries. An entry listed in more than one
// layer belongs to the first.
func newLayerIndex(layers []Layer) layerIndex {
	x := layerIndex{byEntry: map[string]int{}, rest: len(layers)}
	for i, layer := range layers {
		for _, entry := range layer.Packages {
			if entry == restEntry {
				x.rest = min(x.rest, i)
			} else if _, seen := x.byEntry[entry]; !seen {
				x.byEntry[entry] = i
			}
		}
	}
	return x
}

// of returns the layer of the closest listed path at or above p, or the "*" layer.
func (x layerIndex) of(p string) int {
	for {
		if i, ok := x.byEntry[p]; ok {
			return i
		}
		cut := strings.LastIndex(p, "/")
		if cut <= 1 {
			return x.rest
		}
		p = p[:cut]
	}
}

// bandWarnings explains each layer that gets no band. Most often its packages sit
// inside a folder box another layer owns, which flatten fixes.
func bandWarnings(layers []Layer, flatten []string, nodes []Node) []string {
	if len(layers) == 0 {
		return nil
	}
	idx := newLayerIndex(layers)
	items, in := bandItems(folderTree(nodes, flatten))
	banded := map[int]bool{}
	for _, it := range items {
		banded[idx.of(it.path)] = true
	}
	var out []string
	for i := range layers {
		if !banded[i] {
			out = append(out, noBandWarnings(layers, i, idx, items, in.path, flatten, nodes)...)
		}
	}
	return out
}

// noBandWarnings explains why layer i, which has no item at the band level (the
// folder whose items bands are made of), gets no band:
// one warning per band-level folder holding its packages, naming the folders to
// flatten, or a general one when it lists nothing nested.
func noBandWarnings(layers []Layer, i int, idx layerIndex, items []bandItem, level string, flatten []string, nodes []Node) []string {
	toFlatten := map[string]map[string]bool{} // band-level folder -> folders to dissolve
	lists := false                            // whether it lists any package at all
	for _, entry := range layers[i].Packages {
		matches := func(id string) bool { return entryMatches(entry, id) }
		if entry == restEntry || slices.ContainsFunc(nodes, func(n Node) bool { return matches(n.ID) }) {
			lists = true
		}
		for _, it := range items {
			if !strings.HasPrefix(entry, it.path+"/") || !slices.ContainsFunc(it.ids, matches) {
				continue
			}
			if toFlatten[it.path] == nil {
				toFlatten[it.path] = map[string]bool{}
			}
			// Dissolving every folder below the band level down to the entry's parent
			// brings the entry up to it. Starting from the item's path would miss the
			// outer folders of a collapsed chain like "a/b".
			for p := entry[:strings.LastIndex(entry, "/")]; len(p) > len(level); p = p[:strings.LastIndex(p, "/")] {
				if !slices.Contains(flatten, p) {
					toFlatten[it.path][displayPath(p)] = true
				}
			}
		}
	}
	name := layerLabel(layers, i)
	if len(toFlatten) == 0 {
		if !lists {
			return nil // its entries match nothing, which is reported already
		}
		return []string{fmt.Sprintf("%s gets no band: no item at the band level falls to it", name)}
	}
	var out []string
	for _, top := range sortedKeys(toFlatten) {
		owner := "which no layer lists"
		if o := idx.of(top); o < len(layers) {
			owner = "which " + layerLabel(layers, o) + " owns"
		}
		out = append(out, fmt.Sprintf(
			"%s gets no band: its packages sit inside folder %q, %s (layers never split a folder box); add `flatten: [%s]` to layer what is inside it",
			name, displayPath(top), owner, strings.Join(sortedKeys(toFlatten[top]), ", ")))
	}
	return out
}

// flattenWarnings returns a warning for each flatten entry that dissolves nothing:
// it names no folder, or a package without subfolders.
func flattenWarnings(flatten []string, nodes []Node) []string {
	var out []string
	for _, entry := range flatten {
		if slices.ContainsFunc(nodes, func(n Node) bool { return strings.HasPrefix(n.ID, entry+"/") }) {
			continue
		}
		if slices.ContainsFunc(nodes, func(n Node) bool { return n.ID == entry }) {
			out = append(out, fmt.Sprintf("flatten entry %q is a package with no subfolders, so there is nothing to dissolve", displayPath(entry)))
		} else {
			out = append(out, fmt.Sprintf("flatten entry %q matches no folder", displayPath(entry)))
		}
	}
	return out
}

// duplicateWarnings returns a warning for each entry listed in more than one layer.
func duplicateWarnings(layers []Layer) []string {
	first := map[string]int{}
	var out []string
	for i, layer := range layers {
		for _, entry := range layer.Packages {
			f, seen := first[entry]
			switch {
			case !seen:
				first[entry] = i
			case f != i:
				out = append(out, fmt.Sprintf("%q is listed in both %s and %s; the first one wins",
					displayPath(entry), layerLabel(layers, f), layerLabel(layers, i)))
			}
		}
	}
	return out
}

// layerLabel names layer i in a warning, by its name or, without one, its number.
func layerLabel(layers []Layer, i int) string {
	if layers[i].Name != "" {
		return fmt.Sprintf("layer %q", layers[i].Name)
	}
	return fmt.Sprintf("layer %d", i+1)
}

// displayPath turns a node ID back into the path as written in the config.
func displayPath(id string) string {
	return strings.TrimPrefix(id, "./")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
