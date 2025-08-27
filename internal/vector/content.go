package vector

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/db"
)

// FileContentProvider retrieves code content from the filesystem.
type FileContentProvider struct {
	baseDir     string // Base directory for relative paths
	maxFileSize int64  // Maximum file size to read (bytes)
}

// NewFileContentProvider creates a new file-based content provider.
func NewFileContentProvider(baseDir string, maxFileSize int64) *FileContentProvider {
	if maxFileSize == 0 {
		maxFileSize = 1024 * 1024 // Default 1MB limit
	}

	return &FileContentProvider{
		baseDir:     baseDir,
		maxFileSize: maxFileSize,
	}
}

// GetCodeContent retrieves content for a single code node.
func (fcp *FileContentProvider) GetCodeContent(ctx context.Context, node *db.CodeNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node cannot be nil")
	}

	// Resolve file path
	filePath := fcp.resolvePath(node.Path)

	// Check file exists and size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", filePath)
		}
		return "", fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	// Check file size
	if fileInfo.Size() > fcp.maxFileSize {
		return "", fmt.Errorf("file %s is too large (%d bytes, max %d)", filePath, fileInfo.Size(), fcp.maxFileSize)
	}

	// Read file content
	content, err := fcp.readFileContent(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Extract specific lines if node has line range
	if node.StartLine.Valid && node.EndLine.Valid {
		return fcp.extractLines(content, int(node.StartLine.Int64), int(node.EndLine.Int64))
	}

	return content, nil
}

// GetCodeContents retrieves content for multiple code nodes efficiently.
func (fcp *FileContentProvider) GetCodeContents(ctx context.Context, nodes []*db.CodeNode) ([]string, error) {
	if len(nodes) == 0 {
		return []string{}, nil
	}

	contents := make([]string, len(nodes))

	// Group nodes by file path for efficient reading
	fileGroups := make(map[string][]*nodeWithIndex)
	for i, node := range nodes {
		if node == nil {
			contents[i] = ""
			continue
		}

		filePath := fcp.resolvePath(node.Path)
		fileGroups[filePath] = append(fileGroups[filePath], &nodeWithIndex{
			node:  node,
			index: i,
		})
	}

	// Process each file group
	for filePath, nodeGroup := range fileGroups {
		// Read file once for all nodes
		fileContent, err := fcp.readFileContentSafe(filePath)
		if err != nil {
			// Set error for all nodes in this file
			for _, nodeWithIdx := range nodeGroup {
				contents[nodeWithIdx.index] = fmt.Sprintf("// Error reading file: %v", err)
			}
			continue
		}

		// Extract content for each node
		for _, nodeWithIdx := range nodeGroup {
			if nodeWithIdx.node.StartLine.Valid && nodeWithIdx.node.EndLine.Valid {
				lineContent, err := fcp.extractLines(fileContent, int(nodeWithIdx.node.StartLine.Int64), int(nodeWithIdx.node.EndLine.Int64))
				if err != nil {
					contents[nodeWithIdx.index] = fmt.Sprintf("// Error extracting lines: %v", err)
				} else {
					contents[nodeWithIdx.index] = lineContent
				}
			} else {
				contents[nodeWithIdx.index] = fileContent
			}
		}
	}

	return contents, nil
}

// nodeWithIndex pairs a node with its index for batch processing.
type nodeWithIndex struct {
	node  *db.CodeNode
	index int
}

// resolvePath converts a potentially relative path to an absolute path.
func (fcp *FileContentProvider) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	if fcp.baseDir == "" {
		return path
	}

	return filepath.Join(fcp.baseDir, path)
}

// readFileContent reads the entire content of a file.
func (fcp *FileContentProvider) readFileContent(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// readFileContentSafe reads file content with size and error checking.
func (fcp *FileContentProvider) readFileContentSafe(filePath string) (string, error) {
	// Check file exists and size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Check file size
	if fileInfo.Size() > fcp.maxFileSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", fileInfo.Size(), fcp.maxFileSize)
	}

	return fcp.readFileContent(filePath)
}

// extractLines extracts specific lines from content (1-based line numbers).
func (fcp *FileContentProvider) extractLines(content string, startLine, endLine int) (string, error) {
	if startLine < 1 || endLine < 1 {
		return "", fmt.Errorf("line numbers must be positive (start: %d, end: %d)", startLine, endLine)
	}

	if startLine > endLine {
		return "", fmt.Errorf("start line (%d) cannot be greater than end line (%d)", startLine, endLine)
	}

	lines := strings.Split(content, "\n")

	// Adjust for 0-based indexing
	start := startLine - 1
	end := endLine - 1

	if start >= len(lines) {
		return "", fmt.Errorf("start line %d exceeds file length (%d lines)", startLine, len(lines))
	}

	if end >= len(lines) {
		end = len(lines) - 1
	}

	selectedLines := lines[start : end+1]
	return strings.Join(selectedLines, "\n"), nil
}

// PlaceholderContentProvider provides placeholder content for testing and development.
type PlaceholderContentProvider struct {
	includeMetadata bool
}

// NewPlaceholderContentProvider creates a content provider that generates placeholder content.
func NewPlaceholderContentProvider(includeMetadata bool) *PlaceholderContentProvider {
	return &PlaceholderContentProvider{
		includeMetadata: includeMetadata,
	}
}

// GetCodeContent generates placeholder content for a code node.
func (pcp *PlaceholderContentProvider) GetCodeContent(ctx context.Context, node *db.CodeNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node cannot be nil")
	}

	var content strings.Builder

	if pcp.includeMetadata {
		content.WriteString(fmt.Sprintf("// File: %s\n", node.Path))

		if node.Language.Valid {
			content.WriteString(fmt.Sprintf("// Language: %s\n", node.Language.String))
		}

		if node.Kind.Valid {
			content.WriteString(fmt.Sprintf("// Kind: %s\n", node.Kind.String))
		}

		if node.Symbol.Valid {
			content.WriteString(fmt.Sprintf("// Symbol: %s\n", node.Symbol.String))
		}

		if node.StartLine.Valid && node.EndLine.Valid {
			content.WriteString(fmt.Sprintf("// Lines: %d-%d\n", node.StartLine.Int64, node.EndLine.Int64))
		}

		content.WriteString(fmt.Sprintf("// Node ID: %d\n", node.ID))
		content.WriteString(fmt.Sprintf("// Session: %s\n", node.SessionID))
		content.WriteString("//\n")
	}

	// Generate appropriate placeholder content based on language
	switch {
	case node.Language.Valid && node.Language.String == "go":
		content.WriteString(pcp.generateGoPlaceholder(node))
	case node.Language.Valid && node.Language.String == "javascript":
		content.WriteString(pcp.generateJavaScriptPlaceholder(node))
	case node.Language.Valid && node.Language.String == "python":
		content.WriteString(pcp.generatePythonPlaceholder(node))
	default:
		content.WriteString(pcp.generateGenericPlaceholder(node))
	}

	return content.String(), nil
}

// GetCodeContents generates placeholder content for multiple nodes.
func (pcp *PlaceholderContentProvider) GetCodeContents(ctx context.Context, nodes []*db.CodeNode) ([]string, error) {
	contents := make([]string, len(nodes))

	for i, node := range nodes {
		content, err := pcp.GetCodeContent(ctx, node)
		if err != nil {
			contents[i] = fmt.Sprintf("// Error generating content: %v", err)
		} else {
			contents[i] = content
		}
	}

	return contents, nil
}

// generateGoPlaceholder generates Go-specific placeholder content.
func (pcp *PlaceholderContentProvider) generateGoPlaceholder(node *db.CodeNode) string {
	symbol := "PlaceholderCode"
	if node.Symbol.Valid {
		symbol = node.Symbol.String
	}

	if node.Kind.Valid {
		switch node.Kind.String {
		case "function":
			return fmt.Sprintf("func %s() {\n\t// Placeholder implementation\n\t// TODO: Add actual code content\n}", symbol)
		case "method":
			return fmt.Sprintf("func (r *Receiver) %s() {\n\t// Placeholder implementation\n\t// TODO: Add actual code content\n}", symbol)
		case "type":
			return fmt.Sprintf("type %s struct {\n\t// Placeholder fields\n\t// TODO: Add actual type definition\n}", symbol)
		case "interface":
			return fmt.Sprintf("type %s interface {\n\t// Placeholder methods\n\t// TODO: Add actual interface definition\n}", symbol)
		case "variable":
			return fmt.Sprintf("var %s = \"placeholder value\"\n// TODO: Add actual variable value", symbol)
		case "constant":
			return fmt.Sprintf("const %s = \"placeholder constant\"\n// TODO: Add actual constant value", symbol)
		}
	}

	return fmt.Sprintf("// Placeholder Go code for %s\n// TODO: Add actual implementation", symbol)
}

// generateJavaScriptPlaceholder generates JavaScript-specific placeholder content.
func (pcp *PlaceholderContentProvider) generateJavaScriptPlaceholder(node *db.CodeNode) string {
	symbol := "placeholderCode"
	if node.Symbol.Valid {
		symbol = node.Symbol.String
	}

	if node.Kind.Valid {
		switch node.Kind.String {
		case "function":
			return fmt.Sprintf("function %s() {\n  // Placeholder implementation\n  // TODO: Add actual code content\n}", symbol)
		case "method":
			return fmt.Sprintf("  %s() {\n    // Placeholder implementation\n    // TODO: Add actual code content\n  }", symbol)
		case "class":
			return fmt.Sprintf("class %s {\n  // Placeholder class\n  // TODO: Add actual class definition\n}", symbol)
		case "variable":
			return fmt.Sprintf("const %s = 'placeholder value';\n// TODO: Add actual variable value", symbol)
		}
	}

	return fmt.Sprintf("// Placeholder JavaScript code for %s\n// TODO: Add actual implementation", symbol)
}

// generatePythonPlaceholder generates Python-specific placeholder content.
func (pcp *PlaceholderContentProvider) generatePythonPlaceholder(node *db.CodeNode) string {
	symbol := "placeholder_code"
	if node.Symbol.Valid {
		symbol = node.Symbol.String
	}

	if node.Kind.Valid {
		switch node.Kind.String {
		case "function":
			return fmt.Sprintf("def %s():\n    \"\"\"Placeholder implementation\"\"\"\n    # TODO: Add actual code content\n    pass", symbol)
		case "method":
			return fmt.Sprintf("    def %s(self):\n        \"\"\"Placeholder implementation\"\"\"\n        # TODO: Add actual code content\n        pass", symbol)
		case "class":
			return fmt.Sprintf("class %s:\n    \"\"\"Placeholder class\"\"\"\n    # TODO: Add actual class definition\n    pass", symbol)
		case "variable":
			return fmt.Sprintf("%s = 'placeholder value'\n# TODO: Add actual variable value", symbol)
		}
	}

	return fmt.Sprintf("# Placeholder Python code for %s\n# TODO: Add actual implementation", symbol)
}

// generateGenericPlaceholder generates generic placeholder content.
func (pcp *PlaceholderContentProvider) generateGenericPlaceholder(node *db.CodeNode) string {
	symbol := "placeholder_code"
	if node.Symbol.Valid {
		symbol = node.Symbol.String
	}

	return fmt.Sprintf("/* Placeholder code for %s */\n/* TODO: Add actual implementation */\n/* Language: %s */",
		symbol,
		func() string {
			if node.Language.Valid {
				return node.Language.String
			}
			return "unknown"
		}())
}
