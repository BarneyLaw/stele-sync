package annot

import "strings"

// The profile is functions returning values, not mutable package-level maps.
func kinds() []Kind { return []Kind{Strokes, EraserPaths, TextItems, Shapes, ImageItems} }
func kindIndex(k Kind) int {
	for i, v := range kinds() {
		if k == v {
			return i
		}
	}
	return -1
}
func topLevelKeys() []string {
	return []string{"version", "sourceFile", "sourcePdf", "updatedAt", "strokes", "eraserPaths", "textItems", "shapes", "imageItems", "pdfPageTemplates", "nativePageTemplatesEditable", "appendedPages", "deletedPdfPages", "permanentlyDeletedPdfPages", "removedPages"}
}
func structural(key string) bool { return key == "appendedPages" || key == "removedPages" }
func reserved(key string) bool {
	return kindIndex(Kind(key)) >= 0 || key == "pdfPageTemplates" || key == "deletedPdfPages" || key == "permanentlyDeletedPdfPages"
}
func compareElement(a, b ElementKey) int {
	if a.Kind != b.Kind {
		return kindIndex(a.Kind) - kindIndex(b.Kind)
	}
	return strings.Compare(a.ID, b.ID)
}
func compareEntry(a, b EntryKey) int {
	if a.Family != b.Family {
		return strings.Compare(string(a.Family), string(b.Family))
	}
	if a.Page < b.Page {
		return -1
	}
	if a.Page > b.Page {
		return 1
	}
	return 0
}

// RequiresWholeDocument quarantines added pages and their trash: numeric page
// references shift, and restoring trash intentionally reuses annotation ids.
func RequiresWholeDocument(sc Sidecar) bool {
	for _, key := range []string{"appendedPages", "removedPages"} {
		if raw, ok := sc.meta[key]; ok && string(raw) != "[]" {
			return true
		}
	}
	return false
}
