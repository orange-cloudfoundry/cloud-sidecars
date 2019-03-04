package sidecars

import (
	"github.com/ArthurHlt/zipper"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
)

func DownloadSidecar(dir string, c *config.SidecarConfig, forceDl bool) error {
	entry := log.WithField("component", "Downloader").WithField("sidecar", c.Name)
	isEmpty, err := IsEmptyDir(dir)
	if err != nil {
		return err
	}
	if !isEmpty && !forceDl {
		entry.Infof("Skipping downloading from %s (directory not empty, sidecar must be already downloaded)", c.ArtifactURL)
		return nil
	}
	if !isEmpty {
		err := os.RemoveAll(dir)
		if err != nil {
			return err
		}
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	entry.Infof("Downloading from %s ...", c.ArtifactURL)
	err = DownloadArtifact(dir, c.ArtifactURL, c.ArtifactType)
	if err != nil {
		return err
	}
	entry.Infof("Finished downloading from %s ...", c.ArtifactURL)
	return nil
}

func DownloadArtifact(dir, uri, fileType string) error {
	var s *zipper.Session
	var err error
	if fileType != "" {
		s, err = zipper.CreateSession(uri, fileType)
	} else {
		s, err = zipper.CreateSession(uri)
	}
	if err != nil {
		return err
	}

	zipFile, err := s.Zip()
	if err != nil {
		return err
	}

	zipLocal, err := ioutil.TempFile("", "downloads-sidecar")
	if err != nil {
		zipFile.Close()
		return err
	}
	defer func() {
		zipLocal.Close()
		os.Remove(zipLocal.Name())
	}()

	_, err = io.Copy(zipLocal, zipFile)
	if err != nil {
		zipFile.Close()
		return err
	}
	zipFile.Close()

	uz := NewUnzip(zipLocal.Name(), dir)
	err = uz.Extract()
	if err != nil {
		return err
	}
	return nil
}

func IsEmptyDir(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}
