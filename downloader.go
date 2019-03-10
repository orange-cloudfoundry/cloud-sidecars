package sidecars

import (
	"fmt"
	"github.com/ArthurHlt/zipper"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
)

func DownloadSidecar(dir string, c *config.Sidecar) error {
	entry := log.WithField("component", "Downloader").WithField("sidecar", c.Name)
	entry.Infof("Downloading from %s ...", c.ArtifactURI)
	err := DownloadArtifact(dir, c.ArtifactURI, c.ArtifactType, c.ArtifactSha1)
	if err != nil {
		return err
	}
	entry.Infof("Finished downloading from %s ...", c.ArtifactURI)
	return nil
}

func DownloadArtifact(dir, uri, fileType, sha1 string) error {
	s, err := ZipperSess(uri, fileType)
	if err != nil {
		return err
	}

	if sha1 != "" {
		isDiff, cSha1, err := s.IsDiff(sha1)
		if err != nil {
			return err
		}
		if isDiff {
			return fmt.Errorf("Sha1 '%s' mismatch with current sha1 '%s'.", sha1, cSha1)
		}
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

func DiffSha1(s *zipper.Session, storedSha1 string) bool {
	isDiff, _, _ := s.IsDiff(storedSha1)
	return isDiff
}

func ZipperSess(uri, fileType string) (*zipper.Session, error) {
	if fileType != "" {
		return zipper.CreateSession(uri, fileType)
	}
	return zipper.CreateSession(uri)
}
