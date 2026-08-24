package processing

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/schema"
)

// A nil type-ID set allows every observable type and avoids map lookup during default enrichment.
type observableTypeSelector struct {
	typeIDs map[int64]struct{}
}

func newObservableTypeSelector(compiled *schema.Compiled, typeIDs []int64) (observableTypeSelector, error) {
	if len(typeIDs) == 0 {
		return observableTypeSelector{}, nil
	}

	selected := make(map[int64]struct{}, len(typeIDs))
	unknownSet := make(map[int64]struct{})
	for _, typeID := range typeIDs {
		if _, present := compiled.ObservableTypes[typeID]; !present {
			unknownSet[typeID] = struct{}{}
			continue
		}
		selected[typeID] = struct{}{}
	}
	if len(unknownSet) > 0 {
		unknownIDs := make([]int64, 0, len(unknownSet))
		for typeID := range unknownSet {
			unknownIDs = append(unknownIDs, typeID)
		}
		slices.Sort(unknownIDs)
		unknown := make([]string, len(unknownIDs))
		for index, typeID := range unknownIDs {
			unknown[index] = strconv.FormatInt(typeID, 10)
		}
		return observableTypeSelector{}, fmt.Errorf(
			"enrichment processor has unknown observable type IDs: %s",
			strings.Join(unknown, ", "),
		)
	}

	return observableTypeSelector{typeIDs: selected}, nil
}

func (s observableTypeSelector) allows(typeID int64) bool {
	if s.typeIDs == nil {
		return true
	}
	_, present := s.typeIDs[typeID]
	return present
}
