package structs

import (
	"bytes"
	"encoding/gob"
)

func DeepCopy(src any, dist any) error {
	buf := bytes.Buffer{}

	err := gob.NewEncoder(&buf).Encode(src)
	if err != nil {
		return err
	}

	err = gob.NewDecoder(&buf).Decode(dist)
	if err != nil {
		return err
	}

	return nil
}
