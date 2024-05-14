package sidecars

import (
	"fmt"
	"github.com/ArthurHlt/zipper"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
)

func DownloadSidecar(zipFilePath string, c *config.Sidecar) error {
	entry := log.WithField("component", "Downloader").WithField("sidecar", c.Name)
	entry.Infof("Downloading from %s ...", c.ArtifactURI)
	err := DownloadArtifact(zipFilePath, c.ArtifactURI, c.ArtifactType, c.ArtifactSha1)
	if err != nil {
		return err
	}
	entry.Infof("Finished downloading from %s ...", c.ArtifactURI)
	return nil
}

func DownloadArtifact(zipFilePath, uri, fileType, sha1 string) error {
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
			return fmt.Errorf("SHA1 '%s' mismatch with current sha1 '%s'", sha1, cSha1)
		}
	}

	zipFile, err := s.Zip()
	if err != nil {
		return err
	}

	zipLocal, err := os.Create(zipFilePath)
	if err != nil {
		zipFile.Close()
		return err
	}
	defer zipLocal.Close()

	_, err = io.Copy(zipLocal, zipFile)
	if err != nil {
		zipFile.Close()
		return err
	}
	zipFile.Close()

	return nil
}

func ZipperSess(uri, fileType string) (*zipper.Session, error) {
	if fileType != "" {
		return zipper.CreateSession(uri, fileType)
	}
	return zipper.CreateSession(uri)
}
