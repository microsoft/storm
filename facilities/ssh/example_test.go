package ssh_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	remotessh "github.com/microsoft/storm/facilities/ssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func ExampleClient() {
	key, err := remotessh.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}
	// Install key.AuthorizedKey when provisioning this disposable guest.
	cfg := remotessh.Config{
		Addr:             "192.0.2.10:22",
		User:             "tester",
		Auth:             key.Signer,
		HostKey:          remotessh.InsecureHostKey(),
		RetryAuthFailure: true,
	}
	bootCtx, stopBoot := context.WithTimeout(context.Background(), 3*time.Minute)
	client, err := remotessh.WaitReady(bootCtx, cfg, time.Second)
	stopBoot() // The established client is independent of bootCtx.
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := client.Run(ctx, "uname -r")
	if err != nil {
		log.Fatal(err)
	}
	if result.ExitCode != 0 {
		log.Fatalf("exit %d: %s", result.ExitCode, result.Stderr)
	}
	fmt.Print(string(result.Stdout))

	result, err = client.RunIO(ctx, "cat", strings.NewReader("input\n"), os.Stdout, os.Stderr)
	if err != nil {
		log.Fatal(err)
	}
	if result.ExitCode != 0 {
		log.Fatalf("streamed command exited %d", result.ExitCode)
	}
	if err := client.Upload(ctx, "/tmp/settings.json", strings.NewReader(`{"enabled":true}`)); err != nil {
		log.Fatal(err)
	}
	var release bytes.Buffer
	if err := client.Download(ctx, "/etc/os-release", &release); err != nil {
		log.Fatal(err)
	}
	fmt.Print(release.String())
}

func ExampleClient_SudoSFTP() {
	privateKey, err := os.ReadFile("run-key")
	if err != nil {
		log.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		log.Fatal(err)
	}
	verifyHost, err := knownhosts.New("known_hosts")
	if err != nil {
		log.Fatal(err)
	}
	cfg := remotessh.Config{
		Addr:    "192.0.2.10:22",
		User:    "tester",
		Auth:    signer,
		HostKey: verifyHost,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, err := remotessh.Dial(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	files, err := client.SudoSFTP()
	if err != nil {
		log.Fatal(err)
	}
	if err := files.Download(ctx, "/var/log/messages", os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_WaitDisconnected() {
	key, err := remotessh.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}
	// Install key.AuthorizedKey when provisioning this disposable guest.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, err := remotessh.Dial(ctx, remotessh.Config{
		Addr: "192.0.2.10:22", User: "tester", Auth: key.Signer,
		HostKey: remotessh.InsecureHostKey(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	result, err := client.Run(ctx, "sudo -n reboot")
	var missing *ssh.ExitMissingError
	if err != nil && !errors.As(err, &missing) {
		log.Fatal(err)
	}
	if err == nil && result.ExitCode != 0 {
		log.Fatalf("reboot exited %d: %s", result.ExitCode, result.Stderr)
	}
	if err := client.WaitDisconnected(ctx); err != nil {
		log.Fatal(err)
	}
	// Reconnect with WaitReady and verify the guest booted; disconnect alone
	// does not prove a successful reboot.
}

func ExampleSFTP() {
	key, err := remotessh.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}
	// Install key.AuthorizedKey when provisioning this disposable guest.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, err := remotessh.Dial(ctx, remotessh.Config{
		Addr: "192.0.2.10:22", User: "tester", Auth: key.Signer,
		HostKey: remotessh.InsecureHostKey(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// An explicit path skips discovery; omitting it enables auto-discovery.
	files, err := client.SudoSFTP("/opt/ssh/sftp-server")
	if err != nil {
		log.Fatal(err)
	}
	if err := files.Upload(ctx, "/etc/example.json", strings.NewReader(`{"enabled":true}`)); err != nil {
		log.Fatal(err)
	}
	if err := files.Download(ctx, "/etc/example.json", os.Stdout); err != nil {
		log.Fatal(err)
	}
}
