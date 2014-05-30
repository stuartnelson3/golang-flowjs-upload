package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sort"
	"strconv"
	"strings"
)

var completedFiles = make(chan string, 100)

func main() {
	for i := 0; i < 3; i++ {
		go assembleFile(completedFiles)
	}

	m := http.NewServeMux()

	m.Handle("/upload", streamHandler(uploadHandler))

	m.HandleFunc("/public/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, r.URL.Path[1:])
	})

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(404)
			return
		}
		f, _ := os.Open("./index.html")
		body, _ := ioutil.ReadAll(f)
		w.Write(body)
	})

	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()
	http.ListenAndServe(":3000", m)
}

type ByChunk []os.FileInfo

func (a ByChunk) Len() int      { return len(a) }
func (a ByChunk) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByChunk) Less(i, j int) bool {
	ai, _ := strconv.Atoi(a[i].Name())
	aj, _ := strconv.Atoi(a[j].Name())
	return ai < aj
}

func uploadHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "POST" {
		return streamingReader(w, r)
	} else if r.Method == "GET" {
		return continueUpload(w, r)
	} else {
		return errors.New("Not found")
	}
}

type streamHandler func(http.ResponseWriter, *http.Request) error

func (fn streamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

func continueUpload(w http.ResponseWriter, r *http.Request) error {
	chunkDirPath := "./incomplete/" + r.FormValue("flowFilename") + "/" + r.FormValue("flowChunkNumber")
	if _, err := os.Stat(chunkDirPath); err != nil {
		return err
	}
	return nil
}

func streamingReader(w http.ResponseWriter, r *http.Request) error {
	buf := new(bytes.Buffer)
	reader, err := r.MultipartReader()
	// Part 1: Chunk Number
	// Part 6: File Name
	// Part 8: Total Chunks
	// Part 9: Chunk Data
	if err != nil {
		return err
	}

	part, err := reader.NextPart()
	if err != nil {
		return err
	}
	io.Copy(buf, part)
	chunkNo := buf.String()
	buf.Reset()

	for i := 0; i < 5; i++ {
		// move through unused parts
		part, err = reader.NextPart()
		if err != nil {
			return err
		}
	}

	io.Copy(buf, part)
	fileName := buf.String()
	buf.Reset()

	for i := 0; i < 2; i++ {
		// move through unused parts
		part, err = reader.NextPart()
		if err != nil {
			return err
		}
	}
	io.Copy(buf, part)
	chunkTotal := buf.String()
	buf.Reset()

	part, err = reader.NextPart()
	if err != nil {
		return err
	}

	chunkDirPath := "./incomplete/" + fileName
	err = os.MkdirAll(chunkDirPath, 02750)
	if err != nil {
		return err
	}

	dst, err := os.Create(chunkDirPath + "/" + chunkNo)
	if err != nil {
		return err
	}
	defer dst.Close()
	io.Copy(dst, part)

	fileInfos, err := ioutil.ReadDir(chunkDirPath)
	if err != nil {
		return err
	}

	cT, err := strconv.Atoi(chunkTotal)
	if err != nil {
		return err
	}
	if len(fileInfos) == cT {
		completedFiles <- chunkDirPath
	}
	return nil
}

func chunkedReader(w http.ResponseWriter, r *http.Request) error {
	r.ParseMultipartForm(25)

	chunkDirPath := "./incomplete/" + r.FormValue("flowFilename")
	err := os.MkdirAll(chunkDirPath, 02750)
	if err != nil {
		return err
	}

	for _, fileHeader := range r.MultipartForm.File["file"] {
		src, err := fileHeader.Open()
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(chunkDirPath + "/" + r.FormValue("flowChunkNumber"))
		if err != nil {
			return err
		}
		defer dst.Close()
		io.Copy(dst, src)

		fileInfos, err := ioutil.ReadDir(chunkDirPath)
		if err != nil {
			return err
		}

		cT, err := strconv.Atoi(r.FormValue("flowTotalChunks"))
		if err != nil {
			return err
		}

		if len(fileInfos) == cT {
			completedFiles <- chunkDirPath
		}
	}
	return nil
}

func assembleFile(jobs <-chan string) {
	for path := range jobs {
		fileInfos, err := ioutil.ReadDir(path)
		if err != nil {
			return
		}

		// create final file to write to
		dst, err := os.Create(strings.Split(path, "/")[2])
		if err != nil {
			return
		}
		defer dst.Close()

		sort.Sort(ByChunk(fileInfos))
		for _, fs := range fileInfos {
			src, err := os.Open(path + "/" + fs.Name())
			if err != nil {
				return
			}
			defer src.Close()
			io.Copy(dst, src)
		}
		os.RemoveAll(path)
	}
}
