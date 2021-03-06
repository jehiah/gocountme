package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
)

const BytesUint64 = 8
const Hash_Max = float64(1<<64 - 1)

var Default_KMinValues_Size = 1 << 10

func HashUint64ToBytes(hash uint64) []byte {
	hashBytes := new(bytes.Buffer)
	binary.Write(hashBytes, binary.BigEndian, hash)
	return hashBytes.Bytes()
}

func HashBytesToUint64(hashBytes []byte) uint64 {
	// TODO: error checking here
	var hash uint64
	hashReader := bytes.NewBuffer(hashBytes)
	binary.Read(hashReader, binary.BigEndian, &hash)
	return hash
}

func Union(others ...*KMinValues) *KMinValues {
	maxsize := smallestK(others...)
	maxlen := 0
	idxs := make([]int, len(others))
	for i, other := range others {
		if maxlen < other.Len() {
			maxlen = other.Len()
		}
		idxs[i] = other.Len() - 1
	}

	// We directly create a kminvalues object here so that we can have raw be
	// pre-initialized with nil values
	newkmv := &KMinValues{
		Raw:     make([]byte, maxlen*BytesUint64, maxsize*BytesUint64),
		MaxSize: maxsize,
	}

	var kmin, kminTmp []byte
	jmin := make([]int, 0, len(others))
	for i := maxlen - 1; i >= 0; i-- {
		kmin = nil
		jmin = jmin[:0]
		for j, other := range others {
			kminTmp = other.GetHashBytes(idxs[j])
			if kminTmp != nil {
				if kmin == nil || kminTmp != nil && bytes.Compare(kmin, kminTmp) > 0 {
					kmin = kminTmp
					jmin = jmin[:0]
					jmin = append(jmin, j)
				} else if kmin != nil && bytes.Equal(kmin, kminTmp) {
					jmin = append(jmin, j)
				}
			}
		}
		for _, j := range jmin {
			idxs[j]--
		}
		if kmin != nil {
			newkmv.SetHash(i, kmin)
		}
	}
	return newkmv
}

func cardinality(maxSize int, kMin uint64) float64 {
	return float64(maxSize-1.0) * Hash_Max / float64(kMin)
}

func smallestK(others ...*KMinValues) int {
	minsize := others[0].MaxSize
	for _, other := range others[1:] {
		if minsize > other.MaxSize {
			minsize = other.MaxSize
		}
	}
	return minsize
}

type KMinValues struct {
	Raw     []byte
	MaxSize int
}

func (kmv *KMinValues) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	N := kmv.Len()
	fmt.Fprintf(&buffer, `{"k":%d, "data":[`, kmv.MaxSize)
	for n := 0; n < N; n++ {
		if n == N-1 {
			fmt.Fprintf(&buffer, "%d]}", kmv.GetHash(n))
		} else {
			fmt.Fprintf(&buffer, "%d,", kmv.GetHash(n))
		}
	}
	return buffer.Bytes(), nil
}

func NewKMinValues(capacity int) *KMinValues {
	return &KMinValues{
		Raw:     make([]byte, 0, capacity*BytesUint64),
		MaxSize: capacity,
	}
}

func KMinValuesFromBytes(raw []byte) *KMinValues {
	if len(raw) == 0 {
		return NewKMinValues(Default_KMinValues_Size)
	}
	buf := bytes.NewBuffer(raw)

	var maxSizeTmp uint64
	var maxSize int
	err := binary.Read(buf, binary.BigEndian, &maxSizeTmp)
	if err != nil {
		log.Println("error reading size")
		return NewKMinValues(Default_KMinValues_Size)
	}
	maxSize = int(maxSizeTmp)

	kmv := KMinValues{
		Raw:     raw[BytesUint64:],
		MaxSize: maxSize,
	}
	return &kmv
}

func (kmv *KMinValues) GetHash(i int) uint64 {
	hashBytes := kmv.Raw[i*BytesUint64 : (i+1)*BytesUint64]
	return HashBytesToUint64(hashBytes)
}

func (kmv *KMinValues) GetHashBytes(i int) []byte {
	if i < 0 || i >= kmv.Len() {
		return nil
	}
	return kmv.Raw[i*BytesUint64 : (i+1)*BytesUint64]
}

func (kmv *KMinValues) Bytes() []byte {
	sizeBytes := make([]byte, BytesUint64, BytesUint64+len(kmv.Raw))
	binary.BigEndian.PutUint64(sizeBytes, uint64(kmv.MaxSize))
	result := append(sizeBytes, kmv.Raw...)
	return result
}

func (kmv *KMinValues) Len() int { return len(kmv.Raw) / BytesUint64 }

func (kmv *KMinValues) SetHash(i int, hash []byte) {
	ib := i * BytesUint64
	copy(kmv.Raw[ib:], hash)
}

func (kmv *KMinValues) FindHash(hash uint64) int {
	hashBytes := HashUint64ToBytes(hash)
	return kmv.FindHashBytes(hashBytes)
}

func (kmv *KMinValues) FindHashBytes(hash []byte) int {
	idx, found := kmv.LocateHashBytes(hash)
	if found {
		return idx
	}
	return -1
}

func (kmv *KMinValues) LocateHashBytes(hash []byte) (int, bool) {
	found := sort.Search(kmv.Len(), func(i int) bool { return bytes.Compare(kmv.GetHashBytes(i), hash) <= 0 })
	if found < kmv.Len() && bytes.Equal(kmv.GetHashBytes(found), hash) {
		return found, true
	}
	return found, false
}

func (kmv *KMinValues) AddHash(hash uint64) bool {
	hashBytes := HashUint64ToBytes(hash)
	return kmv.AddHashBytes(hashBytes)
}

func (kmv *KMinValues) popSet(idx int, hash []byte) {
	ib := idx * BytesUint64
	copy(kmv.Raw[:ib-BytesUint64], kmv.Raw[BytesUint64:ib])
	copy(kmv.Raw[ib-BytesUint64:], hash)
}

func (kmv *KMinValues) insert(idx int, hash []byte) {
	ib := idx * BytesUint64
	kmv.Raw = append(kmv.Raw, make([]byte, BytesUint64)...)
	copy(kmv.Raw[ib+BytesUint64:], kmv.Raw[ib:])
	copy(kmv.Raw[ib:], hash)
}

// Adds a hash to the KMV and maintains the sorting of the values.
// Furthermore, we make sure that items we are inserting are unique by
// searching for them prior to insertion.  We wait to do this seach last
// because it is computationally expensive so we attempt to throw away the hash
// in every way possible before performing it.
func (kmv *KMinValues) AddHashBytes(hash []byte) bool {
	n := kmv.Len()
	if n >= kmv.MaxSize {
		if bytes.Compare(kmv.GetHashBytes(0), hash) < 0 {
			return false
		}
		idx, found := kmv.LocateHashBytes(hash)
		if !found {
			kmv.popSet(idx, hash)
		} else {
			return false
		}
	} else {
		idx, found := kmv.LocateHashBytes(hash)
		if !found {
			if cap(kmv.Raw) == len(kmv.Raw)+1 {
				kmv.increaseCapacity(len(kmv.Raw) * 2)
			}
			kmv.insert(idx, hash)
		} else {
			return false
		}
	}
	return true
}

// Adds extra capacity to the underlying []uint64 array that stores the hashes
func (kmv *KMinValues) increaseCapacity(newcap int) bool {
	N := cap(kmv.Raw)
	if newcap < N {
		return false
	}
	if newcap/BytesUint64 > kmv.MaxSize {
		if N == kmv.MaxSize*BytesUint64 {
			return false
		}
		newcap = kmv.MaxSize * BytesUint64
	}
	newarray := make([]byte, len(kmv.Raw), newcap)
	copy(newarray[:len(kmv.Raw)], kmv.Raw)
	kmv.Raw = newarray
	return true
}

func (kmv *KMinValues) Cardinality() float64 {
	if kmv.Len() < kmv.MaxSize {
		return float64(kmv.Len())
	}
	return cardinality(kmv.MaxSize, kmv.GetHash(0))
}

func (kmv *KMinValues) CardinalityIntersection(others ...*KMinValues) float64 {
	X, n := DirectSum(append(others, kmv)...)
	return float64(n) / float64(X.MaxSize) * X.Cardinality()

}

func (kmv *KMinValues) CardinalityUnion(others ...*KMinValues) float64 {
	X, _ := DirectSum(append(others, kmv)...)
	return X.Cardinality()

}

func (kmv *KMinValues) Jaccard(others ...*KMinValues) float64 {
	X, n := DirectSum(append(others, kmv)...)
	return float64(n) / float64(X.MaxSize)
}

// Returns a new KMinValues object is the union between the current and the
// given objects
func (kmv *KMinValues) Union(others ...*KMinValues) *KMinValues {
	return Union(append(others, kmv)...)
}

func (kmv *KMinValues) RelativeError() float64 {
	return math.Sqrt(2.0 / (math.Pi * float64(kmv.MaxSize-2)))
}

func DirectSum(others ...*KMinValues) (*KMinValues, int) {
	n := 0
	X := Union(others...)
	// TODO: can we optimize this loop somehow?
	var found bool
	for i := 0; i < X.Len(); i++ {
		xHash := X.GetHashBytes(i)
		found = true
		for _, other := range others {
			if other.FindHashBytes(xHash) < 0 {
				found = false
				break
			}
		}
		if found {
			n += 1
		}
	}
	return X, n
}
