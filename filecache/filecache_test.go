package filecache

import (
	"reflect"
	"testing"
)

func assert(t *testing.T, got, exp interface{}) {
	if !reflect.DeepEqual(got, exp) {
		t.Fatalf(`failed
     got: %+v
expected: %+v
 `, got, exp)
	}
}

func TestBasic(t *testing.T) {
	c := New(10)
	c.Add("a", "foo.txt", "", []byte{1, 1, 1})
	assert(t, c.Size(), int64(3))
	assert(t, c.Keys(), []string{"a"})
	assert(t, c.Get("a"), &Entry{"foo.txt", "", []byte{1, 1, 1}})

	c.Add("b", "foo.txt", "", []byte{2, 2, 2})
	assert(t, c.Size(), int64(6))
	assert(t, len(c.Keys()), 2)

	c.Add("b", "foo.txt", "", []byte{3, 3})
	assert(t, c.Size(), int64(5))
	assert(t, c.Keys(), []string{"a", "b"})

	c.Add("c", "foo.txt", "", []byte{4, 4, 4, 4, 4, 4})
	assert(t, c.Size(), int64(8))
	assert(t, c.Keys(), []string{"b", "c"})
	var nile *Entry = nil
	assert(t, c.Get("a"), nile)
}
