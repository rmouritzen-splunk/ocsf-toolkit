package schema

import (
	"cmp"
	"math"
	"slices"
	"strconv"
)

const numericEnumScanThreshold = 16

type numericEnumIndex struct {
	minimum       int64
	maximum       int64
	dense         []*EnumDefinition
	scan          []numericEnumEntry
	sparse        map[int64]*EnumDefinition
	other         *EnumDefinition
	keysCanonical bool
}

type numericEnumEntry struct {
	value      int64
	definition *EnumDefinition
}

// NumericEnumDefinition returns the definition indexed for an integral enum value. The owning compiled schema must
// have initialized its traversal cache before this method is used.
func (a *ItemAttributeDefinition) NumericEnumDefinition(value int64) *EnumDefinition {
	if a == nil || a.numericEnums == nil {
		return nil
	}
	index := a.numericEnums
	if value == 99 {
		return index.other
	}
	if index.dense != nil {
		if value < index.minimum || value > index.maximum {
			return nil
		}
		return index.dense[value-index.minimum]
	}
	for _, entry := range index.scan {
		if entry.value == value {
			return entry.definition
		}
	}
	return index.sparse[value]
}

// NumericEnumKeysCanonical reports whether every numeric enum key is a canonical signed 64-bit integer. When
// true, an exact json.Number spelling may be looked up directly in Enum before numeric normalization.
func (a *ItemAttributeDefinition) NumericEnumKeysCanonical() bool {
	return a != nil && a.numericEnums != nil && a.numericEnums.keysCanonical
}

func (s *Compiled) initializeItemNumericEnums(item *ItemDefinition) {
	if item == nil {
		return
	}
	for _, attribute := range item.Attributes {
		if attribute == nil || attribute.Enum == nil ||
			(attribute.PrimitiveType != "integer_t" && attribute.PrimitiveType != "long_t") {
			continue
		}
		entries := make([]numericEnumEntry, 0, len(attribute.Enum))
		var other *EnumDefinition
		keysCanonical := true
		for text, definition := range attribute.Enum {
			value, err := strconv.ParseInt(text, 10, 64)
			if err != nil || strconv.FormatInt(value, 10) != text {
				keysCanonical = false
				continue
			}
			if value == 99 {
				other = definition
			} else {
				entries = append(entries, numericEnumEntry{value: value, definition: definition})
			}
		}
		index := &numericEnumIndex{other: other, keysCanonical: keysCanonical}
		if len(entries) > 0 {
			slices.SortFunc(entries, func(left, right numericEnumEntry) int {
				return cmp.Compare(left.value, right.value)
			})
			contiguous := true
			for position := 1; position < len(entries); position++ {
				if entries[position-1].value == math.MaxInt64 ||
					entries[position].value != entries[position-1].value+1 {
					contiguous = false
					break
				}
			}
			switch {
			case contiguous:
				index.minimum = entries[0].value
				index.maximum = entries[len(entries)-1].value
				index.dense = make([]*EnumDefinition, len(entries))
				for position, entry := range entries {
					index.dense[position] = entry.definition
				}
			case len(entries) <= numericEnumScanThreshold:
				index.scan = entries
			default:
				index.sparse = make(map[int64]*EnumDefinition, len(entries))
				for _, entry := range entries {
					index.sparse[entry.value] = entry.definition
				}
			}
		}
		attribute.numericEnums = index
	}
}
