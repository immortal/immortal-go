// +build freebsd netbsd openbsd dragonfly darwin

package immortal

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScanDir struct
type ScanDir struct {
	scandir   string
	sdir      string
	services  map[string]string
	watchDir  chan struct{}
	watchFile chan string
}

// NewScanDir returns ScanDir struct
func NewScanDir(path string) (*ScanDir, error) {
	if info, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("%q no such file or directory", path)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", path)
	}

	dir, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	d, err := os.Open(dir)
	if err != nil {
		if os.IsPermission(err) {
			return nil, os.ErrPermission
		}
		return nil, err
	}
	defer d.Close()

	return &ScanDir{
		scandir:   dir,
		sdir:      GetSdir(),
		services:  map[string]string{},
		watchDir:  make(chan struct{}, 1),
		watchFile: make(chan string, 1),
	}, nil
}

// Start check for changes on directory
func (s *ScanDir) Start(ctl Control) {
	log.Printf("immortal scandir: %s", s.scandir)
	s.watchDir <- struct{}{}
	for {
		select {
		case <-s.watchDir:
			log.Printf("Starting scaning= %s\n", s.scandir)
			if err := s.Scandir(s.scandir); err != nil && !os.IsPermission(err) {
				log.Fatal(err)
			}
			go WatchDir(s.scandir, s.watchDir)
		case file := <-s.watchFile:
			serviceFile := filepath.Base(file)
			serviceName := strings.TrimSuffix(serviceFile, filepath.Ext(serviceFile))
			if isFile(file) {
				md5, err := md5sum(file)
				if err != nil {
					log.Fatalf("Error getting the md5sum: %s", err)
				}
				// restart if file changed
				if md5 != s.services[serviceName] {
					s.services[serviceName] = md5
					log.Printf("Restarting (halt): %s socket: %s\n", serviceName, filepath.Join(s.sdir, serviceName, "immortal.sock"))
				}
				log.Printf("Starting: %s socket: %s\n", serviceName, filepath.Join(s.sdir, serviceName, "immortal.sock"))
				go WatchFile(file, s.watchFile)
			} else {
				// remove service
				log.Printf("Exiting: %s socket: %s\n", serviceName, filepath.Join(s.sdir, serviceName, "immortal.sock"))
				delete(s.services, serviceName)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Scaner searches for *.yml if file changes it will reload(stop-start)
func (s *ScanDir) Scandir(dir string) error {
	find := func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if f.Mode().IsRegular() {
			if filepath.Ext(f.Name()) == ".yml" {
				name := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
				md5, err := md5sum(path)
				if err != nil {
					return fmt.Errorf("Error getting the md5sum: %s", err)
				}
				if _, ok := s.services[name]; !ok {
					s.services[name] = md5
					log.Printf("Starting service: %s socket: %s\n", name, filepath.Join(s.sdir, name, "immortal.sock"))
					go WatchFile(path, s.watchFile)
				}
			}
		}
		return err
	}
	return filepath.Walk(s.scandir, find)
}
