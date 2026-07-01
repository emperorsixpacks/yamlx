package yamlx

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const defaultMaxDepth = 10

var (
	fileCache   = make(map[string][]byte)
	parsedCache = make(map[string]*yaml.Node)
	cacheMu     sync.RWMutex
)

// resolveIncludes walks a yaml.Node tree and resolves any nodes tagged with !include.
func resolveIncludes(node *yaml.Node) error {
	basePath, _ := os.Getwd()
	seen := make(map[string]bool)
	return resolveIncludesWalk(node, basePath, seen, 0, defaultMaxDepth)
}

// resolveIncludesWalk walks the AST and resolves !include tags.
func resolveIncludesWalk(node *yaml.Node, basePath string, seen map[string]bool, depth, maxDepth int) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := resolveIncludesWalk(child, basePath, seen, depth, maxDepth); err != nil {
				return err
			}
		}

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			val := node.Content[i+1]
			if err := resolveIncludesWalk(val, basePath, seen, depth, maxDepth); err != nil {
				return err
			}
		}

	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := resolveIncludesWalk(child, basePath, seen, depth, maxDepth); err != nil {
				return err
			}
		}

	case yaml.ScalarNode:
		if node.Tag == "!include" {
			return resolveIncludeNode(node, basePath, seen, depth, maxDepth)
		}
	}

	return nil
}

// resolveIncludeNode replaces a !include node with the content of the referenced file.
func resolveIncludeNode(node *yaml.Node, basePath string, seen map[string]bool, depth, maxDepth int) error {
	if depth >= maxDepth {
		return NewIncludeError(node.Value, "depth")
	}

	rawPath := node.Value
	var incPath, mode, fallback string

	if idx := strings.Index(rawPath, ":-"); idx != -1 {
		incPath = rawPath[:idx]
		mode = "default"
		fallback = rawPath[idx+2:]
	} else if idx := strings.Index(rawPath, ":?"); idx != -1 {
		incPath = rawPath[:idx]
		mode = "required"
	} else {
		incPath = rawPath
	}

	absPath, err := filepath.Abs(filepath.Join(basePath, incPath))
	if err != nil {
		if mode == "default" {
			return resolveIncludeFile(node, fallback, basePath, seen, depth, maxDepth)
		}
		return NewIncludeError(incPath, "not_found")
	}

	if seen[absPath] {
		return NewIncludeError(incPath, "cycle")
	}
	seen[absPath] = true

	data, err := readFileCached(absPath)
	if err != nil {
		delete(seen, absPath)
		if mode == "default" {
			return resolveIncludeFile(node, fallback, basePath, seen, depth, maxDepth)
		}
		return NewIncludeError(incPath, "not_found")
	}

	return loadIncludedContent(node, data, absPath, seen)
}

// resolveIncludeFile loads a YAML file by path and replaces the node.
func resolveIncludeFile(node *yaml.Node, filePath, basePath string, seen map[string]bool, depth, maxDepth int) error {
	if depth >= maxDepth {
		return NewIncludeError(filePath, "depth")
	}

	absPath, err := filepath.Abs(filepath.Join(basePath, filePath))
	if err != nil {
		return NewIncludeError(filePath, "not_found")
	}

	if seen[absPath] {
		return NewIncludeError(filePath, "cycle")
	}
	seen[absPath] = true

	data, err := readFileCached(absPath)
	if err != nil {
		delete(seen, absPath)
		return NewIncludeError(filePath, "not_found")
	}

	return loadIncludedContent(node, data, absPath, seen)
}

// loadIncludedContent parses YAML data, resolves nested includes, and replaces the node.
func loadIncludedContent(node *yaml.Node, data []byte, absPath string, seen map[string]bool) error {
	included, err := parseCached(absPath, data)
	if err != nil {
		return err
	}

	incBase := filepath.Dir(absPath)
	if err := resolveIncludesWalk(included, incBase, seen, 1, defaultMaxDepth); err != nil {
		return err
	}

	delete(seen, absPath)

	if included.Kind == yaml.DocumentNode && len(included.Content) == 1 {
		*node = *included.Content[0]
	} else {
		*node = *included
	}

	return nil
}

// readFileCached reads a file and caches its contents.
func readFileCached(path string) ([]byte, error) {
	cacheMu.RLock()
	data, ok := fileCache[path]
	cacheMu.RUnlock()
	if ok {
		return data, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cacheMu.Lock()
	fileCache[path] = data
	cacheMu.Unlock()

	return data, nil
}

// parseCached parses YAML data and caches the parsed node.
func parseCached(path string, data []byte) (*yaml.Node, error) {
	cacheMu.RLock()
	node, ok := parsedCache[path]
	cacheMu.RUnlock()
	if ok {
	 copy := *node
		return &copy, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	cacheMu.Lock()
	parsedCache[path] = &doc
	cacheMu.Unlock()

	copy := doc
	return &copy, nil
}

// ResetCache clears the file and parsed caches. Useful for tests.
func ResetCache() {
	cacheMu.Lock()
	fileCache = make(map[string][]byte)
	parsedCache = make(map[string]*yaml.Node)
	cacheMu.Unlock()
}
