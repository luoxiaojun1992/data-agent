package workspace

//go:generate mockery --name WorkspaceManager --output ./mocks --outpkg mocks

// WorkspaceManager defines the workspace file management contract.
type WorkspaceManager interface {
	ReadFile(userID, sessionID, filename string) ([]byte, error)
	WriteFile(userID, sessionID, filename string, data []byte) error
	List(userID, sessionID string) ([]string, error)
}
