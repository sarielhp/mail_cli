package outlook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/label"
)

type graphFolder struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	UnreadItemCount  int64  `json:"unreadItemCount"`
	TotalItemCount   int64  `json:"totalItemCount"`
	ChildFolderCount int    `json:"childFolderCount"`
}

func (c *client) fetchFoldersRecursive(parentID string, parentPath string) ([]cfg_acc.Item, map[string]string, error) {
	var urlStr string
	if parentID == "" {
		urlStr = "https://graph.microsoft.com/v1.0/me/mailFolders?$top=250"
	} else {
		urlStr = fmt.Sprintf("https://graph.microsoft.com/v1.0/me/mailFolders/%s/childFolders?$top=250", parentID)
	}

	resp, err := c.client.Get(urlStr)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return nil, nil, fmt.Errorf("fetching folders failed: status %d: %s", resp.StatusCode, b)
	}

	var res struct {
		Value []graphFolder `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, nil, err
	}

	items := []cfg_acc.Item{}
	idMap := make(map[string]string)

	for _, f := range res.Value {
		fullName := f.DisplayName
		if parentPath != "" {
			fullName = parentPath + "/" + f.DisplayName
		}
		items = append(items, cfg_acc.Item{
			Name:           f.DisplayName,
			FullName:       fullName,
			MessagesTotal:  f.TotalItemCount,
			MessagesUnread: f.UnreadItemCount,
			IsLabel:        true,
		})
		idMap[strings.ToLower(fullName)] = f.ID
		idMap[strings.ToLower(f.DisplayName)] = f.ID

		if f.ChildFolderCount > 0 {
			childItems, childMap, err := c.fetchFoldersRecursive(f.ID, fullName)
			if err == nil {
				items = append(items, childItems...)
				for k, v := range childMap {
					idMap[k] = v
				}
			}
		}
	}

	return items, idMap, nil
}

func (c *client) getFolderID(folderName string) (string, error) {
	_, idMap, err := c.fetchFoldersRecursive("", "")
	if err != nil {
		return "", err
	}
	id, exists := idMap[strings.ToLower(folderName)]
	if !exists {
		return "", fmt.Errorf("folder %q not found", folderName)
	}
	return id, nil
}

func (c *client) GetLabelItems() ([]cfg_acc.Item, error) {
	if err := c.init(); err != nil {
		return nil, err
	}
	items, _, err := c.fetchFoldersRecursive("", "")
	return items, err
}

func (c *client) GetMatchingLabels(prefix string) ([]string, error) {
	// Every entry point that reaches Graph has to run init first: the HTTP
	// client is built lazily, and a freshly constructed client -- one the
	// TUI just switched to, with Validate never called -- has a nil one.
	if err := c.init(); err != nil {
		return nil, err
	}
	items, _, err := c.fetchFoldersRecursive("", "")
	if err != nil {
		return nil, err
	}
	var matched []string
	prefixLower := strings.ToLower(prefix)
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.FullName), prefixLower) {
			matched = append(matched, item.FullName)
		}
	}
	return matched, nil
}

func (c *client) EnsureLabelExists(folder label.Folder) error {
	name := folder.String()
	if err := c.init(); err != nil {
		return err
	}

	_, idMap, err := c.fetchFoldersRecursive("", "")
	if err != nil {
		return err
	}

	if _, exists := idMap[strings.ToLower(name)]; exists {
		return nil
	}

	// Support creation of subfolders
	parts := strings.Split(name, "/")
	currentPath := ""
	parentID := ""

	for _, part := range parts {
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath = currentPath + "/" + part
		}

		id, exists := idMap[strings.ToLower(currentPath)]
		if exists {
			parentID = id
			continue
		}

		// Create this folder level
		if err := label.ValidateNewName(currentPath); err != nil {
			return err
		}
		createdID, err := c.createFolderLevel(parentID, part)
		if err != nil {
			return err
		}

		parentID = createdID
		idMap[strings.ToLower(currentPath)] = createdID
	}

	return nil
}

// createFolderLevel creates one folder level via the Graph API and returns
// its new ID.
//
// It used to be inlined in EnsureLabelExists's loop with `defer
// resp.Body.Close()`, which schedules the close for the *function's* return,
// not the loop iteration's -- so every earlier level's response body stayed
// open until the whole path finished creating. Bounded by path depth today,
// it is the pattern that becomes FD/socket exhaustion the moment this loop
// walks a longer path (L4). A defer scoped to this function's own return
// closes each response before the next iteration opens another.
func (c *client) createFolderLevel(parentID, part string) (string, error) {
	var urlStr string
	if parentID == "" {
		urlStr = "https://graph.microsoft.com/v1.0/me/mailFolders"
	} else {
		urlStr = fmt.Sprintf("https://graph.microsoft.com/v1.0/me/mailFolders/%s/childFolders", parentID)
	}

	bodyBytes, _ := json.Marshal(map[string]string{
		"displayName": part,
	})

	resp, err := c.client.Post(urlStr, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create folder level %s: %w", part, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return "", fmt.Errorf("failed to create folder level %s, status %d: %s", part, resp.StatusCode, b)
	}

	var created graphFolder
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (c *client) RenameLabel(oldFolder label.Folder, newFolder label.Folder) error {
	oldName := oldFolder.String()
	newName := newFolder.String()
	if err := label.ValidateNewName(newName); err != nil {
		return err
	}
	if err := c.init(); err != nil {
		return err
	}

	id, err := c.getFolderID(oldName)
	if err != nil {
		return err
	}

	// Rename only renames the local display name of that specific folder
	parts := strings.Split(newName, "/")
	displayPart := parts[len(parts)-1]

	bodyBytes, _ := json.Marshal(map[string]string{
		"displayName": displayPart,
	})

	req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("https://graph.microsoft.com/v1.0/me/mailFolders/%s", id), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return fmt.Errorf("rename failed: status %d: %s", resp.StatusCode, b)
	}

	return nil
}

func (c *client) DeleteLabel(folder label.Folder) error {
	name := folder.String()
	if err := c.init(); err != nil {
		return err
	}

	id, err := c.getFolderID(name)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("https://graph.microsoft.com/v1.0/me/mailFolders/%s", id), nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return fmt.Errorf("delete failed: status %d: %s", resp.StatusCode, b)
	}

	return nil
}

func (c *client) FixLabels() error {
	// Ensure standard folders exist
	standardFolders := []string{"Inbox", "Archive", "Junk Email", "Sent Items", "Deleted Items"}
	for _, folder := range standardFolders {
		_ = c.EnsureLabelExists(label.FromServer(folder))
	}
	return nil
}
