package yamlx

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultMaxDepth = 10

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
// Supports: !include ./file.yaml, !include ./file.yaml:? (required), !include ./file.yaml:-fallback.yaml
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

	data, err := os.ReadFile(absPath)
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

	data, err := os.ReadFile(absPath)
	if err != nil {
		delete(seen, absPath)
		return NewIncludeError(filePath, "not_found")
	}

	return loadIncludedContent(node, data, absPath, seen)
}

// loadIncludedContent parses YAML data, resolves nested includes, and replaces the node.
func loadIncludedContent(node *yaml.Node, data []byte, absPath string, seen map[string]bool) error {
	var included yaml.Node
	if err := yaml.Unmarshal(data, &included); err != nil {
		return err
	}

	incBase := filepath.Dir(absPath)
	if err := resolveIncludesWalk(&included, incBase, seen, 1, defaultMaxDepth); err != nil {
		return err
	}

	delete(seen, absPath)

	if included.Kind == yaml.DocumentNode && len(included.Content) == 1 {
		*node = *included.Content[0]
	} else {
		*node = included
	}

	return nil
}
