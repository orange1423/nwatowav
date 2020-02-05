package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/orange1423/nwa"
)

var inputfile = flag.String("inputfile", "", "path to the input file.")
var outputpath = flag.String("outputpath", "", "path to the output file.")
var logger *log.Logger

type fileType int

const (
	NONE fileType = iota
	NWA
	NWK
	OVK
)

func main() {
	flag.Parse()
	exepath, err := os.Executable()
	if err != nil {
		log.Fatalln("fail to create log path!")
	}
	dir := filepath.Dir(exepath)
	logfile, err := os.OpenFile(dir+"\\log\\"+time.Now().Format("2006-01-02")+".log", os.O_APPEND|os.O_CREATE, 666)
	if err != nil {
		log.Fatalln("fail to create log file!")
	}
	logger = log.New(logfile, "", log.LstdFlags|log.Lshortfile)
	if *inputfile == "" {
		logger.Fatal("You need to define an input file!")
	}
	if *outputpath == "" {
		logger.Fatal("You need to define an output file!")
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	file, err := os.Open(*inputfile)
	defer file.Close()
	if err != nil {
		logger.Fatal(err)
	}

	var outfilename, outext, outpath string
	var filetype fileType
	var headblksz int64

	switch {
	case strings.Contains(*inputfile, ".nwa"):
		{
			filetype = NWA
			outext = "wav"
		}
	case strings.Contains(*inputfile, ".nwk"):
		{
			filetype = NWK
			headblksz = 12
			outext = "wav"
		}
	case strings.Contains(*inputfile, ".ovk"):
		{
			filetype = OVK
			headblksz = 16
			outext = "ogg"
		}
	}
	if filetype == NONE {
		logger.Fatal("This program can only handle .nwa/.nwk/.ovk files right now.")
	}

	var fileName string
	fileName = strings.Split(filepath.Base(*inputfile), ".")[0]
	outfilename = *outputpath + fileName

	if filetype == NWA {
		var data io.Reader
		if data, err = nwa.NewNwaFile(file); err != nil {
			logger.Fatal(err)
		}

		outpath = fmt.Sprintf("%s.%s", outfilename, outext)

		var out *os.File
		out, err = os.Create(outpath)
		if err != nil {
			logger.Fatal(err)
		}
		defer out.Close()

		if _, err = io.Copy(out, data); err != nil {
			logger.Fatal(err)
		}
	} else { // NWK or OVK files
		var indexcount int32
		binary.Read(file, binary.LittleEndian, &indexcount)
		if indexcount <= 0 {
			if filetype == OVK {
				logger.Fatalf("Invalid Ogg-ovk file: %s: index = %d\n", inputfile, indexcount)
			} else {
				logger.Fatalf("Invalid Koe-nkw file: %s: index = %d\n", inputfile, indexcount)
			}
		}

		tblsiz := make([]int32, indexcount)
		tbloff := make([]int32, indexcount)
		tblcnt := make([]int32, indexcount)
		tblorigsiz := make([]int32, indexcount)

		var i int32
		for i = 0; i < indexcount; i++ {
			buffer := new(bytes.Buffer)
			if count, err := io.CopyN(buffer, file, headblksz); count != headblksz || err != nil {
				logger.Fatal("Couldn't read the index entries!")
			}
			binary.Read(buffer, binary.LittleEndian, &tblsiz[i])
			binary.Read(buffer, binary.LittleEndian, &tbloff[i])
			binary.Read(buffer, binary.LittleEndian, &tblcnt[i])
			binary.Read(buffer, binary.LittleEndian, &tblorigsiz[i])
		}

		c := make(chan int, indexcount)
		for i = 0; i < indexcount; i++ {
			if tbloff[i] <= 0 || tblsiz[i] <= 0 {
				logger.Fatalf("Invalid table[%d]: cnt %d, off %d, size %d\n", i, tblcnt[i], tbloff[i], tblsiz[i])
				continue
			}
			outpath = fmt.Sprintf("%s-%d.%s", outfilename, tblcnt[i], outext)
			go doDecode(filetype, outpath, *inputfile, tbloff[i], tblsiz[i], c)
		}
		for i = 0; i < indexcount; i++ {
			<-c
		}
	}
}

func doDecode(filetype fileType, filename string, datafile string, offset int32, size int32, c chan int) {
	var count int64
	var data io.Reader

	file, err := os.Open(datafile)
	defer file.Close()
	if err != nil {
		logger.Fatal(err)
	}

	buffer := new(bytes.Buffer)
	file.Seek(int64(offset), 0)
	if count, err = io.CopyN(buffer, file, int64(size)); count != int64(size) || err != nil {
		logger.Fatalf("Couldn't read the data for filename %s: off %d, size %d. Error: %s\n", filename, offset, size, err)
	}

	if filetype == NWK {
		if data, err = nwa.NewNwaFile(buffer); err != nil {
			logger.Fatal(err)
		}
	} else {
		data = buffer
	}

	out, err := os.Create(filename)
	if err != nil {
		logger.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, data); err != nil {
		logger.Fatal(err)
	}
	c <- 1
}
