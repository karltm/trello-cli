package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Scale-Flow/trello-cli/internal/auth"
	"github.com/Scale-Flow/trello-cli/internal/contract"
	"github.com/Scale-Flow/trello-cli/internal/trello"
	"github.com/spf13/cobra"
)

var attachmentsCmd = &cobra.Command{
	Use:   "attachments",
	Short: "Manage card attachments",
}

var attachmentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List attachments on a card",
	RunE: func(cmd *cobra.Command, args []string) error {
		cardID, _ := cmd.Flags().GetString("card")
		if err := contract.RequireFlag("card", cardID); err != nil {
			return err
		}
		creds, err := auth.RequireAuth(getCredStore(), "default")
		if err != nil {
			return err
		}
		attachments, err := getAPIClient(creds).ListAttachments(cmd.Context(), cardID)
		if err != nil {
			return err
		}
		return output(cmd.OutOrStdout(), attachments)
	},
}

var attachmentsAddFileCmd = &cobra.Command{
	Use:   "add-file",
	Short: "Attach a local file to a card",
	RunE: func(cmd *cobra.Command, args []string) error {
		cardID, _ := cmd.Flags().GetString("card")
		path, _ := cmd.Flags().GetString("path")
		name, _ := cmd.Flags().GetString("name")
		if err := contract.RequireFlag("card", cardID); err != nil {
			return err
		}
		if err := contract.ValidateFilePath(path); err != nil {
			return err
		}
		var namePtr *string
		if name != "" {
			namePtr = &name
		}
		creds, err := auth.RequireAuth(getCredStore(), "default")
		if err != nil {
			return err
		}
		attachment, err := getAPIClient(creds).AddFileAttachment(cmd.Context(), cardID, path, namePtr)
		if err != nil {
			return err
		}
		return output(cmd.OutOrStdout(), attachment)
	},
}

var attachmentsAddURLCmd = &cobra.Command{
	Use:   "add-url",
	Short: "Attach a URL to a card",
	RunE: func(cmd *cobra.Command, args []string) error {
		cardID, _ := cmd.Flags().GetString("card")
		urlStr, _ := cmd.Flags().GetString("url")
		name, _ := cmd.Flags().GetString("name")
		if err := contract.RequireFlag("card", cardID); err != nil {
			return err
		}
		if err := contract.ValidateURL(urlStr); err != nil {
			return err
		}
		var namePtr *string
		if name != "" {
			namePtr = &name
		}
		creds, err := auth.RequireAuth(getCredStore(), "default")
		if err != nil {
			return err
		}
		attachment, err := getAPIClient(creds).AddURLAttachment(cmd.Context(), cardID, urlStr, namePtr)
		if err != nil {
			return err
		}
		return output(cmd.OutOrStdout(), attachment)
	},
}

var attachmentsDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an attachment from a card",
	RunE: func(cmd *cobra.Command, args []string) error {
		cardID, _ := cmd.Flags().GetString("card")
		attachmentID, _ := cmd.Flags().GetString("attachment")
		if err := contract.RequireFlag("card", cardID); err != nil {
			return err
		}
		if err := contract.RequireFlag("attachment", attachmentID); err != nil {
			return err
		}
		creds, err := auth.RequireAuth(getCredStore(), "default")
		if err != nil {
			return err
		}
		if err := getAPIClient(creds).DeleteAttachment(cmd.Context(), cardID, attachmentID); err != nil {
			return err
		}
		return output(cmd.OutOrStdout(), trello.DeleteResult{Deleted: true, ID: attachmentID})
	},
}

var attachmentsDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download an attachment's bytes to a local file",
	RunE: func(cmd *cobra.Command, args []string) error {
		cardID, _ := cmd.Flags().GetString("card")
		attachmentID, _ := cmd.Flags().GetString("attachment")
		out, _ := cmd.Flags().GetString("out")
		if err := contract.RequireFlag("card", cardID); err != nil {
			return err
		}
		if err := contract.RequireFlag("attachment", attachmentID); err != nil {
			return err
		}
		creds, err := auth.RequireAuth(getCredStore(), "default")
		if err != nil {
			return err
		}
		body, att, err := getAPIClient(creds).DownloadAttachment(cmd.Context(), cardID, attachmentID)
		if err != nil {
			return err
		}
		defer body.Close()

		destPath, err := resolveDownloadPath(out, att.Name)
		if err != nil {
			return err
		}
		file, err := os.Create(destPath)
		if err != nil {
			return contract.NewError(contract.FileNotFound, "cannot create file: "+destPath)
		}
		written, err := io.Copy(file, body)
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return contract.NewError(contract.UnknownError, "failed to write file: "+err.Error())
		}
		return output(cmd.OutOrStdout(), trello.DownloadResult{
			ID:    att.ID,
			Name:  att.Name,
			Path:  destPath,
			Bytes: written,
		})
	},
}

// resolveDownloadPath determines where to write the attachment. An empty out
// defaults to the attachment's own name in the current directory; a directory
// out writes the attachment's name inside it. The attachment name is reduced to
// its base to keep server-supplied paths from escaping the target directory.
func resolveDownloadPath(out, attachmentName string) (string, error) {
	safeName := filepath.Base(attachmentName)
	if safeName == "." || safeName == "/" || safeName == "" {
		safeName = "attachment"
	}
	if out == "" {
		return safeName, nil
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return filepath.Join(out, safeName), nil
	}
	return out, nil
}

func init() {
	attachmentsListCmd.Flags().String("card", "", "Card ID")
	attachmentsAddFileCmd.Flags().String("card", "", "Card ID")
	attachmentsAddFileCmd.Flags().String("path", "", "File path")
	attachmentsAddFileCmd.Flags().String("name", "", "Attachment name")
	attachmentsAddURLCmd.Flags().String("card", "", "Card ID")
	attachmentsAddURLCmd.Flags().String("url", "", "Attachment URL")
	attachmentsAddURLCmd.Flags().String("name", "", "Attachment name")
	attachmentsDeleteCmd.Flags().String("card", "", "Card ID")
	attachmentsDeleteCmd.Flags().String("attachment", "", "Attachment ID")
	attachmentsDownloadCmd.Flags().String("card", "", "Card ID")
	attachmentsDownloadCmd.Flags().String("attachment", "", "Attachment ID")
	attachmentsDownloadCmd.Flags().String("out", "", "Output file path or directory (defaults to the attachment name in the current directory)")

	attachmentsCmd.AddCommand(attachmentsListCmd)
	attachmentsCmd.AddCommand(attachmentsAddFileCmd)
	attachmentsCmd.AddCommand(attachmentsAddURLCmd)
	attachmentsCmd.AddCommand(attachmentsDeleteCmd)
	attachmentsCmd.AddCommand(attachmentsDownloadCmd)
	rootCmd.AddCommand(attachmentsCmd)
}
