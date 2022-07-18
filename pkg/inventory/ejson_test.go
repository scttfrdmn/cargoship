package inventory

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEjsonerWriteNil(t *testing.T) {
	var v Inventoryer
	v = &EJSONer{}
	err := v.Write(nil, nil)
	require.Error(t, err)
	require.EqualError(t, err, "writer is nil")
}

func TestEjsonerWriteNilInv(t *testing.T) {
	var v Inventoryer
	v = &EJSONer{}
	err := v.Write(io.Discard, nil)
	require.Error(t, err)
	require.EqualError(t, err, "inventory is nil")
}

func TestEjsonerWrite(t *testing.T) {
	var v Inventoryer
	v = &EJSONer{}
	i := &DirectoryInventory{}
	var w bytes.Buffer
	err := v.Write(&w, i)
	require.NoError(t, err)
	require.Contains(t, w.String(), `"files":`)
}

func TestEjsonerReadNil(t *testing.T) {
	var v Inventoryer
	v = &EJSONer{}
	_, err := v.Read(nil)
	require.Error(t, err)
}

func TestEjsonerRead(t *testing.T) {
	var v Inventoryer
	v = &EJSONer{}
	d := []byte(`{"files":[{"path":"/Users/user/Desktop/example-suitcase/thing","destination":"thing","name":"thing","size":3145728,"mod_time":"0001-01-01T00:00:00Z","is_dir":false,"suitcase_index":1}]}`)
	i, err := v.Read(d)
	require.NoError(t, err)
	require.NotNil(t, i)
	require.Equal(t, 1, len(i.Files))
}

func TestEjsonerReadBadJSON(t *testing.T) {
	var v Inventoryer
	v = &EJSONer{}
	d := []byte(`{`)
	i, err := v.Read(d)
	require.Error(t, err)
	require.Nil(t, i)
}
