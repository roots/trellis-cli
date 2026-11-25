package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/cli"

	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
)

// Secrets live in secrets/<env>.yml.age, encrypted with age to the
// environment's admins (their SSH keys) and access.recipients. These
// commands are thin wrappers over project/secrets.

const secretsHelp = `Usage: trellis converge secrets <subcommand> [<args>]

Each environment's secrets are stored in secrets/<env>.yml.age, encrypted
with age to the environment's access.admins (their ed25519/RSA SSH keys) and
access.recipients (age keys, e.g. for CI). Development secrets are plaintext
in secrets/development.yml.

Decryption uses, in order: $TRELLIS_AGE_KEY, $TRELLIS_AGE_KEY_FILE,
~/.config/trellis/age-key.txt, ~/.ssh/id_ed25519, ~/.ssh/id_rsa.

To see readable diffs of encrypted secrets in git:

  echo 'secrets/*.age diff=trellis-secrets' >> .gitattributes
  git config diff.trellis-secrets.textconv "trellis converge secrets show"
`

func NewConvergeSecretsNamespace() *NamespaceCommand {
	return &NamespaceCommand{HelpText: secretsHelp, SynopsisText: "Manage encrypted secrets"}
}

// secretsCommand is a `trellis converge secrets` subcommand.
type secretsCommand struct {
	ui        cli.Ui
	usage     string
	synopsis  string
	help      string
	min, max  int
	noProject bool
	run       func(ui cli.Ui, p *project.Project, args []string) error
}

func (c *secretsCommand) Run(args []string) int {
	if len(args) < c.min || len(args) > c.max {
		c.ui.Error("Usage: " + c.usage)
		return 1
	}
	var p *project.Project
	if !c.noProject {
		if p = openProject(c.ui); p == nil {
			return 1
		}
	}
	if err := c.run(c.ui, p, args); err != nil {
		c.ui.Error(err.Error())
		return 1
	}
	return 0
}

func (c *secretsCommand) Synopsis() string { return c.synopsis }
func (c *secretsCommand) Help() string     { return "Usage: " + c.usage + "\n\n" + c.help }

func warn(ui cli.Ui, warnings []string) {
	for _, w := range warnings {
		ui.Warn("warning: " + w)
	}
}

func NewConvergeSecretsInitCommand(ui cli.Ui) cli.Command {
	return &secretsCommand{ui: ui, min: 1, max: 1,
		usage:    "trellis converge secrets init <environment>",
		synopsis: "Generate database passwords and WordPress salts",
		help:     "Adds any missing generated secrets for every site. Existing values are never replaced.\n",
		run: func(ui cli.Ui, p *project.Project, args []string) error {
			env := args[0]
			e, err := p.Config.Resolve(env)
			if err != nil {
				return err
			}
			doc, f, err := p.LoadSecrets(env, secretKeys(ui))
			if err != nil {
				return err
			}
			var added []string
			if env != config.DevelopmentEnv {
				ok, err := doc.EnsureAdmin()
				if err != nil {
					return err
				}
				if ok {
					added = append(added, "admin_password")
				}
			}
			for _, s := range e.Sites {
				a, err := doc.EnsureSite(s.Name)
				if err != nil {
					return err
				}
				added = append(added, a...)
			}
			if len(added) == 0 && f.Exists {
				ui.Info("All generated secrets already exist in " + f.Path + ".")
				return nil
			}
			b, err := doc.Marshal()
			if err != nil {
				return err
			}
			path, warnings, err := p.SaveSecrets(env, b)
			warn(ui, warnings)
			if err != nil {
				return err
			}
			ui.Info(fmt.Sprintf("Generated %d secrets in %s.", len(added), path))
			return nil
		},
	}
}

func NewConvergeSecretsEditCommand(ui cli.Ui) cli.Command {
	return &secretsCommand{ui: ui, min: 1, max: 1,
		usage:    "trellis converge secrets edit <environment>",
		synopsis: "Edit secrets in $EDITOR",
		help:     "Decrypts to a private temporary file, opens $VISUAL or $EDITOR, then validates and re-encrypts.\n",
		run: func(ui cli.Ui, p *project.Project, args []string) error {
			env := args[0]
			plain, f, err := p.ReadSecrets(env, secretKeys(ui))
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp("", "trellis-secrets-")
			if err != nil {
				return err
			}
			defer func() { _ = os.RemoveAll(dir) }()
			tmp := filepath.Join(dir, env+".yml")
			if err := os.WriteFile(tmp, plain, 0o600); err != nil {
				return err
			}
			for {
				if err := runEditor(tmp); err != nil {
					return err
				}
				edited, err := os.ReadFile(tmp)
				if err != nil {
					return err
				}
				if bytes.Equal(edited, plain) && f.Exists {
					ui.Info("No changes.")
					return nil
				}
				if _, err := secrets.ParseDoc(edited); err != nil {
					ui.Error("Invalid YAML: " + err.Error())
					if ans, _ := ui.Ask("Edit again? [Y/n]"); strings.HasPrefix(strings.ToLower(ans), "n") {
						return errors.New("secrets not saved")
					}
					continue
				}
				path, warnings, err := p.SaveSecrets(env, edited)
				warn(ui, warnings)
				if err != nil {
					return err
				}
				ui.Info("Saved " + path + ".")
				return nil
			}
		},
	}
}

func runEditor(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// Through sh so EDITOR="code --wait" works.
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q: %w", editor, err)
	}
	return nil
}

func NewConvergeSecretsSetCommand(ui cli.Ui) cli.Command {
	return &secretsCommand{ui: ui, min: 2, max: 3,
		usage:    "trellis converge secrets set <environment> <path> [<value>]",
		synopsis: "Set one secret",
		help: `Sets the secret at <path> (slash-separated, as in secret: references).
Without <value>, it is read from stdin or prompted for, which keeps it out
of your shell history:

  trellis converge secrets set production mail/password
  op read op://Vault/Postmark/password | trellis converge secrets set production mail/password
`,
		run: func(ui cli.Ui, p *project.Project, args []string) error {
			env, path := args[0], secrets.RefPath(args[1])
			var value string
			switch {
			case len(args) == 3:
				value = args[2]
			case isTerminal(os.Stdin):
				v, err := ui.AskSecret("Value for " + args[1] + ":")
				if err != nil {
					return err
				}
				value = v
			default:
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				value = strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
			}
			doc, _, err := p.LoadSecrets(env, secretKeys(ui))
			if err != nil {
				return err
			}
			changed, err := doc.Set(path, value, true)
			if err != nil {
				return err
			}
			if !changed {
				ui.Info("Unchanged.")
				return nil
			}
			b, err := doc.Marshal()
			if err != nil {
				return err
			}
			file, warnings, err := p.SaveSecrets(env, b)
			warn(ui, warnings)
			if err != nil {
				return err
			}
			ui.Info(fmt.Sprintf("Set %s in %s.", args[1], file))
			return nil
		},
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func NewConvergeSecretsShowCommand(ui cli.Ui) cli.Command {
	return &secretsCommand{ui: ui, min: 1, max: 1, noProject: true,
		usage:    "trellis converge secrets show <environment|file>",
		synopsis: "Print decrypted secrets",
		help:     "Prints an environment's (or a file's) decrypted secrets. Also works as a git textconv driver.\n",
		run: func(ui cli.Ui, _ *project.Project, args []string) error {
			arg := args[0]
			var plain []byte
			if fi, err := os.Stat(arg); err == nil && !fi.IsDir() {
				data, err := os.ReadFile(arg)
				if err != nil {
					return err
				}
				plain = data
				if secrets.IsEncrypted(data) {
					if plain, err = secrets.Decrypt(data, secretKeys(ui)); err != nil {
						return fmt.Errorf("%s: %w", arg, err)
					}
				}
			} else {
				p := openProject(ui)
				if p == nil {
					return errors.New("not in a Trellis project")
				}
				if plain, _, err = p.ReadSecrets(arg, secretKeys(ui)); err != nil {
					return err
				}
			}
			_, err := os.Stdout.Write(plain)
			return err
		},
	}
}

func NewConvergeSecretsRekeyCommand(ui cli.Ui) cli.Command {
	return &secretsCommand{ui: ui, min: 1, max: 1,
		usage:    "trellis converge secrets rekey <environment>",
		synopsis: "Re-encrypt secrets to the current admins and recipients",
		help: `Re-encrypts secrets to the environment's current access.admins and
access.recipients. Run it after changing either (and after
` + "`trellis converge lock`" + ` when GitHub keys changed). Also encrypts a
plaintext secrets file.

Removing someone only stops them decrypting future versions: rotate any
secret they could read.
`,
		run: func(ui cli.Ui, p *project.Project, args []string) error {
			env := args[0]
			plain, f, err := p.ReadSecrets(env, secretKeys(ui))
			if err != nil {
				return err
			}
			if !f.Exists {
				return fmt.Errorf("%s has no secrets file (run `trellis converge secrets init %s`)", env, env)
			}
			path, warnings, err := p.SaveSecrets(env, plain)
			warn(ui, warnings)
			if err != nil {
				return err
			}
			e, err := p.Config.Resolve(env)
			if err != nil {
				return err
			}
			ui.Info(fmt.Sprintf("Encrypted %s for: %s", path, strings.Join(append(e.Access.Admins, e.Access.Recipients...), ", ")))
			return nil
		},
	}
}

func NewConvergeSecretsKeygenCommand(ui cli.Ui) cli.Command {
	return &secretsCommand{ui: ui, min: 0, max: 0, noProject: true,
		usage:    "trellis converge secrets keygen",
		synopsis: "Create a personal age key for secrets",
		help: `Creates ~/.config/trellis/age-key.txt and prints its public key. You only
need one if your SSH key can't decrypt (it's ECDSA, a security key, or only
in an SSH agent like 1Password's); add the public key to access.recipients.
For CI, put the key file's contents in $TRELLIS_AGE_KEY.
`,
		run: func(ui cli.Ui, _ *project.Project, _ []string) error {
			path := app_paths.ConfigPath("age-key.txt")
			if b, err := os.ReadFile(path); err == nil {
				pub, err := secrets.RecipientOf(b)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				ui.Info(fmt.Sprintf("%s already exists. Public key:\n\n  %s", path, pub))
				return nil
			}
			identity, pub, err := secrets.NewIdentity()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(identity), 0o600); err != nil {
				return err
			}
			ui.Info(fmt.Sprintf("Created %s. Public key:\n\n  %s\n\nAdd it to access.recipients in trellis.yml, then have an existing admin run\n`trellis converge secrets rekey <environment>`.", path, pub))
			return nil
		},
	}
}
