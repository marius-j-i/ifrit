package node


import (
	_"math/big"
	_"fmt"
	"errors"
)

var (
	errIdNotFound = errors.New("ring id not found")
)


func insert(slice []*ringId, newId *ringId) ([]*ringId, int){
	length := len(slice)
	currIdx := 0
	maxIdx := length - 1

	if maxIdx == -1 {
		slice = append(slice, newId)
		return slice, currIdx
	}

	for {
		if currIdx >= maxIdx {
			slice = append(slice, nil)
			copy(slice[currIdx+1:], slice[currIdx:])

			if slice[currIdx].cmpId(newId) == -1 {
				currIdx += 1
			}
			slice[currIdx] = newId

			return slice, currIdx
		}
		mid := (currIdx + maxIdx) / 2

		if slice[mid].cmpId(newId) == -1 {
			currIdx = mid + 1
		} else {
			maxIdx = mid - 1
		}
	}
}


func search (slice []*ringId, searchId *ringId) (int, error) {
	length := len(slice)
	currIdx := 0
	maxIdx := length - 1

	for {
		if currIdx >= maxIdx {
			if slice[currIdx].cmpId(searchId) == 0 {
				return currIdx, nil
			} else {
				return -1, errIdNotFound
			}

		}
		mid := (currIdx + maxIdx) / 2

		cmp := slice[mid].cmpId(searchId)

		if cmp == -1 {
			currIdx = mid + 1
		} else if cmp == 1 {
			maxIdx = mid - 1
		} else {
			return mid, nil
		}
	}
}
