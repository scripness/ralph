package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const completeSignal = "<promise>COMPLETE</promise>"

func runAmpIteration(cfg ResolvedConfig, prompt string) (output string, complete bool, err error) {
	cmd := exec.Command(cfg.Amp.Command, cfg.Amp.Args...)
	cmd.Dir = cfg.ProjectRoot
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", false, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", false, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", false, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", false, fmt.Errorf("failed to start amp: %w", err)
	}

	// Write prompt to stdin
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, prompt)
	}()

	// Read and display output
	var outputBuilder strings.Builder

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line)
			outputBuilder.WriteString(line + "\n")
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		outputBuilder.WriteString(line + "\n")
	}

	cmd.Wait()

	output = outputBuilder.String()
	complete = strings.Contains(output, completeSignal)
	return output, complete, nil
}

func runVerify(cfg ResolvedConfig) error {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" Ralph Verify Mode")
	fmt.Println(strings.Repeat("=", 60))

	prompt := generateVerifyPrompt(cfg)

	cmd := exec.Command(cfg.Amp.Command, cfg.Amp.Args...)
	cmd.Dir = cfg.ProjectRoot
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	io.WriteString(stdin, prompt)
	stdin.Close()

	return cmd.Wait()
}

func runLoop(cfg ResolvedConfig, noVerify bool) error {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" Ralph - Autonomous Agent Loop")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Max iterations: %d\n", cfg.Iterations)
	fmt.Printf(" PRD: %s\n", cfg.PrdPath)
	fmt.Printf(" Project root: %s\n", cfg.ProjectRoot)
	fmt.Println(strings.Repeat("=", 60))

	runPrompt := generateRunPrompt(cfg)

	for i := 1; i <= cfg.Iterations; i++ {
		fmt.Println()
		fmt.Println(strings.Repeat("=", 60))
		fmt.Printf(" Ralph Iteration %d of %d\n", i, cfg.Iterations)
		fmt.Println(strings.Repeat("=", 60))

		prd, err := loadPRD(cfg.PrdPath)
		if err != nil {
			return err
		}

		nextStory := getNextStory(prd)
		if nextStory == nil {
			fmt.Println("All stories already complete.")
			break
		}

		fmt.Printf("\nNext story: %s - %s\n", nextStory.ID, nextStory.Title)

		_, complete, err := runAmpIteration(cfg, runPrompt)
		if err != nil {
			return err
		}

		if complete {
			// Re-read PRD to verify all stories pass
			updatedPrd, err := loadPRD(cfg.PrdPath)
			if err != nil {
				return err
			}

			var incomplete []UserStory
			for _, s := range updatedPrd.UserStories {
				if !s.Passes {
					incomplete = append(incomplete, s)
				}
			}

			if len(incomplete) > 0 {
				fmt.Println("\nWarning: COMPLETE signal but these stories still fail:")
				for _, s := range incomplete {
					fmt.Printf("  - %s: %s\n", s.ID, s.Title)
				}
				fmt.Println("Continuing...")
				continue
			}

			fmt.Println()
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println(" Ralph completed all stories!")
			fmt.Printf(" Completed at iteration %d of %d\n", i, cfg.Iterations)
			fmt.Println(strings.Repeat("=", 60))

			// Run verification unless --no-verify
			if !noVerify && cfg.Verify {
				fmt.Println("\nRunning verification...")
				runVerify(cfg)

				// Check if verification reset any stories
				verifiedPrd, err := loadPRD(cfg.PrdPath)
				if err != nil {
					return err
				}

				var resetStories []UserStory
				for _, s := range verifiedPrd.UserStories {
					if !s.Passes {
						resetStories = append(resetStories, s)
					}
				}

				if len(resetStories) > 0 {
					fmt.Println()
					fmt.Println(strings.Repeat("=", 60))
					fmt.Println(" Verification found issues - stories reset for retry:")
					for _, s := range resetStories {
						fmt.Printf("  - %s: %s\n", s.ID, s.Title)
					}
					fmt.Println(strings.Repeat("=", 60))
					fmt.Println("\nRestarting loop...")
					continue
				}
			}

			fmt.Println()
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println(" Ralph completed and verified!")
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println("\nResults:")
			fmt.Printf("  - PRD: %s\n", cfg.PrdPath)
			fmt.Println("  - Git log: git log --oneline -20")
			fmt.Println("\nReady to merge.")
			return nil
		}

		fmt.Printf("\nIteration %d complete. Continuing...\n", i)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Ralph reached max iterations (%d)\n", cfg.Iterations)
	fmt.Println(" Not all tasks completed - check prd.json for status")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\nTo continue: ralph run %d\n", cfg.Iterations)
	os.Exit(1)
	return nil
}
