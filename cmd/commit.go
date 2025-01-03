package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const systemMessage = `You are intelligent, helpful and an expert developer, who always gives the correct answer and only does what instructed. You always answer truthfully and don't make things up. (When responding to the following prompt, please make sure to properly style your response using Github Flavored Markdown. Use markdown syntax for things like headings, lists, colored text, code blocks, highlights etc. Make sure not to mention markdown or styling in your actual response.)`

const userMessage = `Suggest a precise and informative commit message based on the following diff. Do not use markdown syntax in your response.

The commit message should have description with a short title that follows emoji commit message format like <emoji> <description>.

Examples:
- :refactor: Change log format for better visibility
- :sparkles: Introduce new logging class

Diff: `

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Prints out the diff in the current directory and suggests commit messages",
	Long: `This command prints out the diff of the changes in the current directory.
It helps you see what changes have been made before committing them. It also suggests commit messages.`,
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := getAPIKey()
		if apiKey == "" {
			fmt.Println("No OpenAI API key found. Either set an OPENAI_TOKEN environment variable or use the lazycommit init command to set it.")
			return
		}

		diff := gitDiff()
		if diff == nil {
			return
		}
		commitMessages := getCommitMessages(diff, apiKey)
		if len(commitMessages) == 0 {
			fmt.Println("An error occurred while getting commit messages.")
			return
		}

		prompt := promptui.Select{
			Label: "Select a commit message",
			Items: commitMessages,
		}

		_, selectedMessage, err := prompt.Run()
		if err != nil {
			fmt.Println("Prompt failed:", err)
			return
		}

		commitChanges(selectedMessage)
	},
}

func getAPIKey() string {
	openAIAPIKey := os.Getenv("OPENAI_TOKEN")
	if os.Getenv("OPENAI_TOKEN") == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Error getting home directory:", err)
			return ""
		}
		configFilePath := filepath.Join(homeDir, ".lazycommit.yaml")
		if _, err := os.Stat(configFilePath); err == nil {
			configFile, err := os.ReadFile(configFilePath)
			if err != nil {
				fmt.Println("Error reading config file:", err)
				return ""
			}
			config := make(map[string]string)
			err = yaml.Unmarshal(configFile, &config)
			if err != nil {
				fmt.Println("Error parsing config file:", err)
				return ""
			}
			openAIAPIKey = config["openai_api_key"]
		}
	}
	return openAIAPIKey
}

func gitDiff() []byte {
	cmd := exec.Command("git", "diff", "--staged")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error running git diff:", err)
		return nil
	}
	return output
}

func getCommitMessages(diff []byte, openAIAPIKey string) []string {
	// Call OpenAI API to get commit messages using gpt-3.5-turbo
	url := "https://api.openai.com/v1/chat/completions"
	requestBody, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{"role": "system", "content": systemMessage},
			{"role": "user", "content": userMessage + string(diff)},
		},
		"max_tokens": 150,
		"n":          5,
	})

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openAIAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		fmt.Println("Error parsing response:", err)
		return nil
	}

	choices, ok := result["choices"].([]interface{})
	if !ok {
		fmt.Println("Error parsing response.")
		return nil
	}

	var messages []string
	for _, choice := range choices {
		text, ok := choice.(map[string]interface{})["message"].(map[string]interface{})["content"].(string)
		if ok {
			messages = append(messages, text)
		}
	}

	return messages
}

func commitChanges(message string) {
	cmd := exec.Command("git", "commit", "-m", message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error committing changes:", err)
		return
	}
	fmt.Println(string(output))
}

func init() {
	rootCmd.AddCommand(commitCmd)
}
