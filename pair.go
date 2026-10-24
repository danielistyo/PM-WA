package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"pm-wa/accounts"
	"pm-wa/bot"
)

// runPair implements `pm-wa pair <name>`: onboarding and recovery are the
// same terminal flow — create the account's session store, print the QR,
// block until paired, exit.
func runPair(args []string) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(os.Stderr, "Usage: pm-wa pair <account-name>")
		os.Exit(2)
	}
	name := strings.TrimSpace(args[0])

	res, err := accounts.Resolve(os.Getenv("WA_ACCOUNTS"), accounts.StatFSState())
	if err != nil {
		slog.Error("failed to resolve accounts", "error", err)
		os.Exit(1)
	}

	var acct *accounts.Account
	for i := range res.Accounts {
		if res.Accounts[i].Name == name {
			acct = &res.Accounts[i]
			break
		}
	}
	if acct == nil {
		names := make([]string, 0, len(res.Accounts))
		for _, a := range res.Accounts {
			names = append(names, a.Name)
		}
		fmt.Fprintf(os.Stderr,
			"account %q is not configured; configured accounts: %s\nAdd the name to WA_ACCOUNTS first, then run the pair command again.\n",
			name, strings.Join(names, ", "))
		os.Exit(1)
	}

	if err := acct.EnsureDir(); err != nil {
		slog.Error("failed to create account data directory", "account", name, "error", err)
		os.Exit(1)
	}

	client, err := bot.NewClient(acct.SessionDBPath)
	if err != nil {
		slog.Error("failed to open session store", "account", name, "error", err)
		os.Exit(1)
	}

	// Recovery path: an account whose session is dead (linked device was
	// logged out) re-pairs with the same subcommand after dropping the old
	// session. Confirm, because this discards the stored device.
	if client.WA.Store.ID != nil {
		if !confirmRePair(name) {
			fmt.Println("Aborted; the existing session was kept.")
			return
		}
		fmt.Printf("Re-pairing account %q; discarding its old session.\n", name)
		if err := client.ResetSession(); err != nil {
			slog.Error("failed to reset session store", "account", name, "error", err)
			os.Exit(1)
		}
		if err := client.CloseStore(); err != nil {
			slog.Warn("failed to close old session store", "account", name, "error", err)
		}
		if client, err = bot.NewClient(acct.SessionDBPath); err != nil {
			slog.Error("failed to reopen session store", "account", name, "error", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Pairing account %q. On the phone that owns the target number:\n",
		name)
	fmt.Println("  WhatsApp > Settings > Linked devices > Link a device, then scan.")
	fmt.Println("This command blocks until pairing completes; press Ctrl+C to cancel.")
	if err := client.PairNew(); err != nil {
		slog.Error("pairing failed", "account", name, "error", err)
		os.Exit(1)
	}

	client.Disconnect()
	fmt.Printf("Account %q paired successfully. Start the bot as usual; it will connect on boot.\n", name)
}

// confirmRePair reads a yes/no confirmation for discarding an account's
// stored session. Anything other than y/yes is treated as no.
func confirmRePair(name string) bool {
	fmt.Printf("Account %q already has a linked session. Re-pairing will discard it.\nDiscard and re-pair? [y/N] ", name)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}
