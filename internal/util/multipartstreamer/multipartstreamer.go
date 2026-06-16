package multipartstreamer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sync"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

type multipartStreamer struct {
	contentType string
	reader      io.Reader

	errMu sync.Mutex
	err   error
}

func NewWithFields(fields map[string]string, fileNames ...string) *multipartStreamer {
	if len(fields) == 0 && len(fileNames) == 0 {
		return &multipartStreamer{
			reader: bytes.NewReader([]byte{}),
		}
	}

	pr, pw := io.Pipe()

	multipartWriter := multipart.NewWriter(pw)

	m := &multipartStreamer{
		reader:      pr,
		contentType: multipartWriter.FormDataContentType(),
	}

	go func() {
		defer func() {
			err := multipartWriter.Close()
			if err != nil {
				m.setErr(err)
			}

			err = pw.Close()
			if err != nil {
				m.setErr(err)
			}
		}()

		for name, field := range fields {
			mw, err := multipartWriter.CreateFormField(name)
			if err != nil {
				m.setErr(err)
				return
			}

			_, err = mw.Write([]byte(field))
			if err != nil {
				m.setErr(err)
				return
			}
		}

		for i, filename := range fileNames {
			func() {
				f, err := os.Open(filename)
				if err != nil {
					m.setErr(err)
					return
				}

				defer func() {
					err := f.Close()
					if err != nil {
						m.setErr(err)
					}
				}()

				fileWriter, err := multipartWriter.CreateFormFile(
					fmt.Sprintf("file%02d", i),
					filepath.Base(filename),
				)
				if err != nil {
					m.setErr(err)
					return
				}

				_, err = file.SafeCopy(fileWriter, f)
				if err != nil {
					m.setErr(err)
					return
				}
			}()
		}
	}()

	return m
}

func New(fileNames ...string) *multipartStreamer {
	return NewWithFields(nil, fileNames...)
}

func (m *multipartStreamer) Read(p []byte) (n int, err error) {
	return m.reader.Read(p)
}

func (m *multipartStreamer) Close() error {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	return m.err
}

func (m *multipartStreamer) ContentType() string {
	return m.contentType
}

func (m *multipartStreamer) setErr(err error) {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	m.err = errors.Join(m.err, err)
}
