package main

import (
	"bytes"
	"github.com/codegangsta/martini"
	"github.com/codegangsta/martini-contrib/render"
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

	m := martini.Classic()
	m.Use(render.Renderer(render.Options{
		Layout:     "layout",
		Delims:     render.Delims{"{[{", "}]}"},
		Extensions: []string{".html"}}))

	m.Get("/", func(r render.Render) {
		r.HTML(200, "index", nil)
	})

	m.Post("/upload", streamingReader)

	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()
	m.Run()
}

type ByChunk []os.FileInfo

func (a ByChunk) Len() int      { return len(a) }
func (a ByChunk) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByChunk) Less(i, j int) bool {
	ai, _ := strconv.Atoi(a[i].Name())
	aj, _ := strconv.Atoi(a[j].Name())
	return ai < aj
}

func streamingReader(w http.ResponseWriter, r *http.Request) {
	buf := new(bytes.Buffer)
	reader, err := r.MultipartReader()
	// Part 1: Chunk Number
	// Part 6: File Name
	// Part 8: Total Chunks
	// Part 9: Chunk Data
	if err != nil {
		w.Write([]byte("Error"))
		w.WriteHeader(500)
		return
	}

	part, err := reader.NextPart()
	if err != nil {
		w.Write([]byte("Error"))
		w.WriteHeader(500)
		return
	}
	io.Copy(buf, part)
	chunkNo := buf.String()
	buf.Reset()

	for i := 0; i < 5; i++ {
		// move through unused parts
		part, err = reader.NextPart()
		if err != nil {
			w.Write([]byte("Error"))
			w.WriteHeader(500)
			return
		}
	}

	io.Copy(buf, part)
	fileName := buf.String()
	buf.Reset()

	for i := 0; i < 2; i++ {
		// move through unused parts
		part, err = reader.NextPart()
		if err != nil {
			w.Write([]byte("Error"))
			w.WriteHeader(500)
			return
		}
	}
	io.Copy(buf, part)
	chunkTotal := buf.String()
	buf.Reset()

	part, err = reader.NextPart()
	if err != nil {
		w.Write([]byte("Error"))
		w.WriteHeader(500)
		return
	}

	chunkDirPath := "./incomplete/" + fileName
	_, err = os.Stat(chunkDirPath)
	if err != nil {
		err := os.MkdirAll(chunkDirPath, 02750)
		if err != nil {
			w.Write([]byte("Error creating tempdir"))
			w.WriteHeader(500)
			return
		}
	}

	dst, err := os.Create(chunkDirPath + "/" + chunkNo)
	if err != nil {
		w.Write([]byte("Error creating file"))
		w.WriteHeader(500)
		return
	}
	defer dst.Close()
	io.Copy(dst, part)

	if chunkNo == chunkTotal {
		completedFiles <- chunkDirPath
	}
}

func chunkedReader(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(25)

	chunkDirPath := "./incomplete/" + r.FormValue("flowFilename")
	_, err := os.Stat(chunkDirPath)
	if err != nil {
		err := os.MkdirAll(chunkDirPath, 02750)
		if err != nil {
			w.Write([]byte("Error creating tempdir"))
			w.WriteHeader(500)
			return
		}
	}

	for _, fileHeader := range r.MultipartForm.File["file"] {
		src, err := fileHeader.Open()
		if err != nil {
			w.Write([]byte("Error opening file"))
			w.WriteHeader(500)
			return
		}
		defer src.Close()

		dst, err := os.Create(chunkDirPath + "/" + r.FormValue("flowChunkNumber"))
		if err != nil {
			w.Write([]byte("Error creating file"))
			w.WriteHeader(500)
			return
		}
		defer dst.Close()
		io.Copy(dst, src)

		if r.FormValue("flowChunkNumber") == r.FormValue("flowTotalChunks") {
			completedFiles <- chunkDirPath
		}
	}
	w.Write([]byte("success"))
	w.WriteHeader(200)
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
