package workspace

import (
	"fmt"
	"sort"
	"strings"
)

// PackageClassification represents the tier classification of a package label.
type PackageClassification struct {
	Label       string // Original label (e.g., "pkg:packages/db")
	PackagePath string // Resolved path (e.g., "packages/db")
	Tier        string // Tier name if tiered, empty if domain/app
	IsDomainApp bool   // True if NOT in any tier
}

// ClassifyPackageLabels extracts all pkg:* labels, resolves to paths,
// and classifies each by tier lookup.
func ClassifyPackageLabels(labels []string, prefix string, tiers map[string][]string, workDir string) ([]PackageClassification, error) {
	// Build reverse map: resolved path -> tier name
	pathToTier := make(map[string]string)
	for tierName, paths := range tiers {
		for _, p := range paths {
			pathToTier[NormalizePackagePath(p)] = tierName
		}
	}

	var classifications []PackageClassification
	for _, label := range labels {
		pkgName, ok := ExtractPackageFromLabel(label, prefix)
		if !ok {
			continue
		}

		// Resolve the package name to a canonical workspace path
		pkgPath, err := ResolvePackagePath(workDir, pkgName)
		if err != nil {
			return nil, fmt.Errorf("label %q: %w", label, err)
		}

		normalized := NormalizePackagePath(pkgPath)
		tier := pathToTier[normalized]

		classifications = append(classifications, PackageClassification{
			Label:       label,
			PackagePath: pkgPath,
			Tier:        tier,
			IsDomainApp: tier == "",
		})
	}

	return classifications, nil
}

// ValidatePackageLabels applies routing rules to classified package labels:
//   - 0 labels → error
//   - >1 domain/app → error (cross-domain)
//   - 1 domain/app → route by that label's path
//   - 0 domain/app → route by first label alphabetically
func ValidatePackageLabels(classifications []PackageClassification) (string, error) {
	if len(classifications) == 0 {
		return "", fmt.Errorf("no package labels found")
	}

	// Separate domain/app from tiered packages
	var domainApps []PackageClassification
	for _, c := range classifications {
		if c.IsDomainApp {
			domainApps = append(domainApps, c)
		}
	}

	switch len(domainApps) {
	case 0:
		// All packages are infrastructure/tiered — route by first alphabetically
		sorted := make([]PackageClassification, len(classifications))
		copy(sorted, classifications)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].PackagePath < sorted[j].PackagePath
		})
		return sorted[0].PackagePath, nil

	case 1:
		return domainApps[0].PackagePath, nil

	default:
		// Multiple domain/app packages — cross-domain error
		var paths []string
		for _, d := range domainApps {
			paths = append(paths, d.PackagePath)
		}
		return "", fmt.Errorf("cross-domain: multiple domain/app packages found (%s); split into separate issues", strings.Join(paths, ", "))
	}
}
