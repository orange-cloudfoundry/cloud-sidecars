package sidecars

import (
	"fmt"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	"gopkg.in/yaml.v2"
	"os"
)

type Index struct {
	Name    string `yaml:"name"`
	ZipFile string `yaml:"zip_file"`
	Uri     string `yaml:"uri"`
	Sha1    string `yaml:"sha1"`
}

func (i Index) IsDiff(sha1 string) bool {
	return sha1 != i.Sha1
}

type Indexer struct {
	indexFile string
	indexes   map[string]Index
}

func NewIndexer(indexFile string) *Indexer {
	indexer := &Indexer{
		indexFile: indexFile,
		indexes:   make(map[string]Index),
	}
	err := indexer.loadIndexes()
	if err != nil {
		panic(err)
	}
	return indexer
}

func (i Indexer) HasIndexFile() bool {
	_, err := os.Stat(i.indexFile)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return true
}

func (i *Indexer) loadIndexes() error {
	if !i.HasIndexFile() {
		return nil
	}
	f, err := os.Open(i.indexFile)
	if err != nil {
		return err
	}
	defer f.Close()
	var idxs []Index
	err = yaml.NewDecoder(f).Decode(&idxs)
	if err != nil {
		return err
	}
	indexes := make(map[string]Index)
	for _, i := range idxs {
		indexes[i.Name] = i
	}
	i.indexes = indexes
	return nil
}

func (i Indexer) Index(sidecar *config.Sidecar) (Index, bool) {
	index, ok := i.indexes[sidecar.Name]
	return index, ok
}

func (i Indexer) IndexToRemove(sidecar []*config.Sidecar) []Index {
	idxs := make([]Index, 0)
	for _, v := range i.indexes {
		toDelete := true
		for _, s := range sidecar {
			if v.Name == s.Name {
				toDelete = false
				break
			}
		}
		if toDelete {
			idxs = append(idxs, v)
		}
	}
	return idxs
}

func (i *Indexer) RemoveIndex(index Index) {
	delete(i.indexes, index.Name)
}

func (i Indexer) Indexes() []Index {
	idxs := make([]Index, 0)
	for _, v := range i.indexes {
		idxs = append(idxs, v)
	}
	return idxs
}

func (i *Indexer) UpdateOrCreateIndex(sidecar *config.Sidecar, zipFile string) error {
	index := Index{
		Name:    sidecar.Name,
		Sha1:    sidecar.ArtifactSha1,
		Uri:     sidecar.ArtifactURI,
		ZipFile: zipFile,
	}
	i.indexes[sidecar.Name] = index
	return nil
}

func (i *Indexer) Store() error {
	f, err := os.Create(i.indexFile)
	if err != nil {
		return err
	}
	defer f.Close()

	idxs := make([]Index, 0)
	for _, v := range i.indexes {
		idxs = append(idxs, v)
	}

	return yaml.NewEncoder(f).Encode(idxs)
}

func (i Indexer) ShouldDownload(sidecar *config.Sidecar) (ok bool, why string) {
	if len(i.indexes) == 0 {
		return true, ""
	}
	if sidecar.ArtifactURI == "" {
		return false, ""
	}
	index, ok := i.indexes[sidecar.Name]
	if !ok {
		return true, ""
	}
	if index.Uri != sidecar.ArtifactURI {
		return true, ""
	}
	if sidecar.ArtifactSha1 != index.Sha1 {
		return false, fmt.Sprintf("Index sha1 '%s' mismatch with current sha1 '%s'.", index.Sha1, sidecar.ArtifactSha1)
	}
	return false, ""
}
