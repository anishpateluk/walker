package walker

import (
	"io"
	"os"
	"path/filepath"

	"code.google.com/p/log4go"
)

// Handler defines the interface for objects that will be set as handlers on a
// FetchManager.
type Handler interface {
	// HandleResponse will be called by fetchers as they make requests.
	// Handlers can do whatever they want with responses. HandleResponse will
	// be called as long as the request successfully reached the remote server
	// and got an HTTP code. This means there should never be a FetchError set
	// on the FetchResults.
	HandleResponse(res *FetchResults)
}

// SimpleWriterHandler just writes returned pages as files locally, naming the
// file after the URL of the request.
type SimpleWriterHandler struct{}

func (h *SimpleWriterHandler) HandleResponse(fr *FetchResults) {
	if fr.ExcludedByRobots {
		log4go.Debug("Excluded by robots.txt, ignoring url: %v", fr.URL)
		return
	}
	if fr.Response.StatusCode < 200 || fr.Response.StatusCode >= 300 {
		log4go.Debug("Returned %v ignoring url: %v", fr.Response.StatusCode, fr.URL)
		return
	}

	path := filepath.Join(fr.URL.Host, fr.URL.RequestURI())
	dir, _ := filepath.Split(path)
	log4go.Debug("Creating dir %v", dir)
	if err := os.MkdirAll(dir, 0777); err != nil {
		log4go.Error(err.Error())
		return
	}

	out, err := os.Create(path)
	log4go.Debug("Creating file %v", path)
	if err != nil {
		log4go.Error(err.Error())
		return
	}
	defer func() {
		log4go.Debug("Closing file %v", path)
		err := out.Close()
		if err != nil {
			log4go.Error(err.Error())
		}
	}()
	log4go.Debug("Copying contents to %v", path)
	_, err = io.Copy(out, fr.Response.Body)
	if err != nil {
		log4go.Error(err.Error())
		return
	}
}
