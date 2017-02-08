package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/fdawg4l/nfs"
	"github.com/fdawg4l/nfs/rpc"
	"github.com/fdawg4l/nfs/util"
)

func main() {
	util.DefaultLogger.SetDebug(true)

	b := strings.Split(os.Args[1], ":")

	host := b[0]
	target := b[1]
	dir := os.Args[2]

	util.Infof("host=%s target=%s dir=%s\n", host, target, dir)

	mount, err := nfs.DialMount("tcp", host)
	if err != nil {
		log.Fatalf("unable to dial MOUNT service: %v", err)
	}
	defer mount.Close()

	auth := &rpc.AUTH_UNIX{
		Stamp:       rand.New(rand.NewSource(time.Now().UnixNano())).Uint32(),
		Machinename: "hasselhoff",
		Uid:         1001,
		Gid:         1001,
		GidLen:      1,
	}

	v, err := mount.Mount(target, auth.Auth())
	if err != nil {
		log.Fatalf("unable to mount volume: %v", err)
	}
	defer v.Close()

	if _, err = v.Mkdir(dir, 0775); err != nil {
		log.Fatalf("mkdir error: %v", err)
	}

	if _, err = v.Mkdir(dir, 0775); err == nil {
		log.Fatalf("mkdir expected error")
	}

	// make a nested dir
	if _, err = v.Mkdir(dir+"/a", 0775); err != nil {
		log.Fatalf("mkdir error: %v", err)
	}

	// make a nested dir
	if _, err = v.Mkdir(dir+"/a/b", 0775); err != nil {
		log.Fatalf("mkdir error: %v", err)
	}

	dirs, err := ls(v, ".")
	if err != nil {
		log.Fatalf("ls: %s", err.Error())
	}

	// check the length.  There should only be 1 entry in the target (aside from . and ..)
	if len(dirs) != 3 {
		log.Fatalf("expected 3 dirs, got %d", len(dirs))
	}

	// 10 MB file
	if err = testFileRW(v, "20mb", 10*1024*1024); err != nil {
		log.Fatalf("fail")
	}

	// 7b file
	if err = testFileRW(v, "7b", 7); err != nil {
		log.Fatalf("fail")
	}

	// should return an error
	if err = v.RemoveAll("7b"); err == nil {
		log.Fatalf("expected a NOTADIR error")
	} else {
		nfserr := err.(*nfs.Error)
		if nfserr.ErrorNum != nfs.NFS3ERR_NOTDIR {
			log.Fatalf("Wrong error")
		}
	}

	if err = v.Remove("7b"); err != nil {
		log.Fatalf("rm(7b) err: %s", err.Error())
	}

	if err = v.Remove("20mb"); err != nil {
		log.Fatalf("rm(20mb) err: %s", err.Error())
	}

	_, _, err = v.Lookup(dir)
	if err != nil {
		log.Fatalf("lookup error: %s", err.Error())
	}

	if _, err = ls(v, "."); err != nil {
		log.Fatalf("ls: %s", err.Error())
	}

	if err = v.RmDir(dir); err == nil {
		log.Fatalf("expected not empty error")
	}

	for _, fname := range []string{"/one", "/two", "/a/one", "/a/two", "/a/b/one", "/a/b/two"} {
		if err = testFileRW(v, dir+fname, 10); err != nil {
			log.Fatalf("fail")
		}
	}

	if err = v.RemoveAll(dir); err != nil {
		log.Fatalf("error removing files: %s", err.Error())
	}

	if err = mount.Unmount(); err != nil {
		log.Fatalf("unable to umount target: %v", err)
	}

	mount.Close()
}

func testFileRW(v *nfs.Target, name string, filesize uint64) error {

	// create a temp file
	f, err := os.Open("/dev/urandom")
	if err != nil {
		util.Errorf("error openning random: %s", err.Error())
		return err
	}

	wr, err := v.OpenFile(name, 0777)
	if err != nil {
		util.Errorf("write fail: %s", err.Error())
		return err
	}

	// calculate the sha
	h := sha256.New()
	t := io.TeeReader(f, h)

	// Copy filesize
	_, err = io.CopyN(wr, t, int64(filesize))
	if err != nil {
		util.Errorf("error copying: %s", err.Error())
		return err
	}
	expectedSum := h.Sum(nil)

	if err = wr.Close(); err != nil {
		util.Errorf("error committing: %s", err.Error())
		return err
	}

	//
	// get the file we wrote and calc the sum
	rdr, err := v.Open(name)
	if err != nil {
		util.Errorf("read error: %v", err)
		return err
	}

	h = sha256.New()
	t = io.TeeReader(rdr, h)

	_, err = ioutil.ReadAll(t)
	if err != nil {
		util.Errorf("readall error: %v", err)
		return err
	}
	actualSum := h.Sum(nil)

	if bytes.Compare(actualSum, expectedSum) != 0 {
		log.Fatalf("sums didn't match. actual=%x expected=%s", actualSum, expectedSum) //  Got=0%x expected=0%x", string(buf), testdata)
	}

	log.Printf("Sums match %x %x", actualSum, expectedSum)
	return nil
}

func ls(v *nfs.Target, path string) ([]*nfs.EntryPlus, error) {
	dirs, err := v.ReadDirPlus(path)
	if err != nil {
		return nil, fmt.Errorf("readdir error: %s", err.Error())
	}

	util.Infof("dirs:")
	for _, dir := range dirs {
		util.Infof("\t%s\t%d:%d\t0%o", dir.FileName, dir.Attr.Attr.UID, dir.Attr.Attr.GID, dir.Attr.Attr.Mode)
	}

	return dirs, nil
}