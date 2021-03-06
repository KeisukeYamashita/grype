package grype

import (
	"fmt"
	"os"
	"strings"

	"github.com/anchore/grype/internal"

	"github.com/anchore/grype/grype/db"
	"github.com/anchore/grype/grype/logger"
	"github.com/anchore/grype/grype/match"
	"github.com/anchore/grype/grype/matcher"
	"github.com/anchore/grype/grype/vulnerability"
	"github.com/anchore/grype/internal/bus"
	"github.com/anchore/grype/internal/log"
	"github.com/anchore/syft/syft"
	"github.com/anchore/syft/syft/distro"
	"github.com/anchore/syft/syft/pkg"
	"github.com/anchore/syft/syft/source"
	"github.com/wagoodman/go-partybus"
)

func Catalog(userImageStr string, scopeOpt source.Scope) (source.Metadata, *pkg.Catalog, distro.Distro, error) {
	// handle explicit sbom input first
	if strings.HasPrefix(userImageStr, "sbom:") {
		// the user has explicitly hinted this is an sbom, if there is an issue return the error
		filepath := strings.TrimPrefix(userImageStr, "sbom:")
		sbomReader, err := os.Open(filepath)
		if err != nil {
			return source.Metadata{}, nil, distro.Distro{}, fmt.Errorf("unable to read sbom: %w", err)
		}
		return syft.CatalogFromJSON(sbomReader)
	} else if internal.IsPipedInput() && userImageStr == "" {
		// the user has not provided an image and stdin is a pipe, assume this to be an explicit sbom case
		return syft.CatalogFromJSON(os.Stdin)
	}

	// the user has not hinted that this may be a sbom, but lets try that first... ignore failures and fallback to syft
	if sbomReader, err := os.Open(userImageStr); err == nil {
		sourceMetadata, catalog, theDistro, err := syft.CatalogFromJSON(sbomReader)
		if err == nil {
			return sourceMetadata, catalog, theDistro, nil
		}
	}

	// attempt to parse as an image (left syft handle this)
	theSource, catalog, theDistro, err := syft.Catalog(userImageStr, scopeOpt)
	if err != nil {
		return source.Metadata{}, nil, distro.Distro{}, err
	}
	return theSource.Metadata, catalog, theDistro, nil
}

func FindVulnerabilities(provider vulnerability.Provider, userImageStr string, scopeOpt source.Scope) (match.Matches, source.Metadata, *pkg.Catalog, error) {
	sourceMetadata, catalog, theDistro, err := Catalog(userImageStr, scopeOpt)
	if err != nil {
		return match.Matches{}, source.Metadata{}, nil, err
	}

	return FindVulnerabilitiesForCatalog(provider, theDistro, catalog), sourceMetadata, catalog, nil
}

func FindVulnerabilitiesForCatalog(provider vulnerability.Provider, d distro.Distro, catalog *pkg.Catalog) match.Matches {
	packages := make([]*pkg.Package, 0)
	for p := range catalog.Enumerate() {
		packages = append(packages, p)
	}
	return FindVulnerabilitiesForPackage(provider, d, packages...)
}

func FindVulnerabilitiesForPackage(provider vulnerability.Provider, d distro.Distro, packages ...*pkg.Package) match.Matches {
	return matcher.FindMatches(provider, d, packages...)
}

func LoadVulnerabilityDb(cfg db.Config, update bool) (vulnerability.Provider, vulnerability.MetadataProvider, error) {
	dbCurator := db.NewCurator(cfg)

	if update {
		_, err := dbCurator.Update()
		if err != nil {
			return nil, nil, err
		}
	}

	store, err := dbCurator.GetStore()
	if err != nil {
		return nil, nil, err
	}

	return vulnerability.NewProviderFromStore(store), vulnerability.NewMetadataStoreProvider(store), nil
}

func SetLogger(logger logger.Logger) {
	log.Log = logger
}

func SetBus(b *partybus.Bus) {
	bus.SetPublisher(b)
}
