package deploy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Source is the commit a deploy builds and where it came from.
type Source struct {
	Commit string
	// From describes it for output, e.g. "main on origin".
	From string
	// Notes are things worth telling the user (unpushed commits, ...).
	Notes []string
}

// Resolve picks the commit to deploy from the site's local clone repo.
//
//   - ref (--ref): exactly that, as the clone knows it (a local branch,
//     tag or commit; unpushed commits included).
//   - In CI: the checked-out commit (HEAD). That is what the workflow
//     fetched for this run, including when the workflow lives in another
//     repository than the site.
//   - Otherwise: branch as it is on the clone's remote, fetched now, so a
//     stale or unpushed local branch is never deployed by accident.
func Resolve(ctx context.Context, repo, ref, branch string, ci bool) (Source, error) {
	switch {
	case ref != "":
		c, err := commitOf(ctx, repo, ref)
		return Source{Commit: c, From: ref}, err
	case ci:
		c, err := commitOf(ctx, repo, "HEAD")
		return Source{Commit: c, From: "the checked-out commit (CI)"}, err
	}
	remote, err := remoteFor(ctx, repo, branch)
	if err != nil {
		return Source{}, err
	}
	if remote == "" {
		c, err := commitOf(ctx, repo, branch)
		return Source{Commit: c, From: branch, Notes: []string{"the clone has no git remote: deploying the local " + branch}}, err
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", repo, "fetch", "--quiet", "--no-tags", remote, "refs/heads/"+branch).CombinedOutput(); err != nil {
		return Source{}, fmt.Errorf("fetch %s from %s: %s (--ref %s deploys your local branch)", branch, remote, strings.TrimSpace(string(out)), branch)
	}
	c, err := commitOf(ctx, repo, "FETCH_HEAD")
	if err != nil {
		return Source{}, err
	}
	s := Source{Commit: c, From: branch + " on " + remote}
	// Local commits that aren't on the remote aren't deployed: say so.
	if _, err := commitOf(ctx, repo, "refs/heads/"+branch); err == nil {
		if n, _ := output(ctx, repo, "git", "rev-list", "--count", c+"..refs/heads/"+branch); n != "" && n != "0" {
			s.Notes = append(s.Notes, fmt.Sprintf("your local %s has %s commit(s) not on %s, which aren't deployed (push them, or --ref %s)", branch, n, remote, branch))
		}
	}
	return s, nil
}

// remoteFor is branch's upstream remote, else origin, else the only
// remote; "" when the clone has none.
func remoteFor(ctx context.Context, repo, branch string) (string, error) {
	if r, _ := output(ctx, repo, "git", "config", "--get", "branch."+branch+".remote"); r != "" && r != "." {
		return r, nil
	}
	out, err := output(ctx, repo, "git", "remote")
	if err != nil {
		return "", fmt.Errorf("%s: not a git repository", repo)
	}
	remotes := strings.Fields(out)
	for _, r := range remotes {
		if r == "origin" {
			return r, nil
		}
	}
	switch len(remotes) {
	case 0:
		return "", nil
	case 1:
		return remotes[0], nil
	}
	return "", errors.New("the clone has several git remotes and " + branch + " tracks none of them: set its upstream, or deploy with --ref")
}

func commitOf(ctx context.Context, repo, ref string) (string, error) {
	c, err := output(ctx, repo, "git", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil || c == "" {
		return "", fmt.Errorf("%s: no commit %q (is it fetched?)", repo, ref)
	}
	return c, nil
}
