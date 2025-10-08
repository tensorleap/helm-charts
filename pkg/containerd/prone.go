package containerd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
)

type imgRow struct {
	Name   string
	Digest string
}

func PruneContainerdExceptImageList(ctx context.Context, dockerName, namespace string, keepImages []string, dryRun bool) error {
	rows, err := listImages(ctx, dockerName, namespace)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		log.Infof("no images found in Containerd")
		return nil
	}

	digestToRefs := map[string][]string{}
	for _, r := range rows {
		dg := strings.ToLower(r.Digest)
		if dg == "" {
			continue
		}
		if _, ok := digestToRefs[dg]; !ok {
			digestToRefs[dg] = []string{}
		}
		if r.Name != "" {
			digestToRefs[dg] = append(digestToRefs[dg], r.Name)
		}
	}

	tagsByImageUrl := buildKeepSets(keepImages)
	deleteDigests := make([]string, 0)
	keepImageDigests := make(map[string]bool, 0)
	for _, r := range rows {
		if r.Name == "" || r.Digest == "" {
			continue
		}
		imageAndTag := strings.Split(r.Name, ":")

		tag, ok := tagsByImageUrl[imageAndTag[0]]

		if ok && tag == imageAndTag[1] {
			keepImageDigests[r.Digest] = true
			continue
		}
	}
	for _, r := range rows {
		if r.Digest == "" {
			continue
		}
		if _, ok := keepImageDigests[r.Digest]; ok {
			continue
		}
		deleteDigests = append(deleteDigests, r.Digest)
	}

	if len(deleteDigests) == 0 {
		log.Infof("nothing to delete from Containerd")
		return nil
	}

	inUse, err := digestsInUse(ctx, dockerName, namespace, deleteDigests)
	if err != nil {
		return err
	}

	if len(inUse) > 0 {
		tmp := deleteDigests[:0]
		for _, dg := range deleteDigests {
			if _, used := inUse[dg]; !used {
				tmp = append(tmp, dg)
			} else {
				log.Warnf("⚠️  in use, will NOT delete: %s", dg)
			}
		}
		deleteDigests = tmp
	}

	if len(deleteDigests) == 0 {
		log.Infof("no deletable digests (all in use)")
		return nil
	}

	args := []string{"exec", dockerName, "ctr", "-n", namespace, "images", "rm"}
	digestRefs := make(map[string]string, 0)
	for _, dg := range deleteDigests {
		refs := digestToRefs[dg]
		for _, ref := range refs {
			digestRefs[ref] = ref
		}
		digestRefs[dg] = dg
	}
	for key := range digestRefs {
		args = append(args, digestRefs[key])
	}

	if dryRun {
		log.Infof("[dry-run] docker %s", strings.Join(args, " "))
		return nil
	}

	log.Infof("Deleting %d images from Containerd", len(deleteDigests))

	cmd := exec.CommandContext(ctx, "docker", args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("delete failed: %v\n%s", err, out.String())
	}
	log.Infof("Deleted %d images from Containerd", len(deleteDigests))
	return nil
}

// --- helpers ---

func listImages(ctx context.Context, dockerName, ns string) ([]imgRow, error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", dockerName, "ctr", "-n", ns, "images", "ls")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list images: %w\n%s", err, out.String())
	}
	lines := linesTrim(out.String())
	if len(lines) <= 1 {
		return nil, nil
	}
	var rows []imgRow
	for i, line := range lines {
		if i == 0 && strings.Contains(line, "REF") { // header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if name == "<none>" || name == "-" {
			name = ""
		}
		var dg string
		for _, f := range fields {
			if strings.HasPrefix(f, "sha256:") {
				dg = f
				break
			}
		}
		rows = append(rows, imgRow{Name: name, Digest: dg})
	}
	return rows, nil
}

func digestsInUse(ctx context.Context, dockerName, ns string, dgs []string) (map[string]struct{}, error) {
	set := make(map[string]struct{})
	first12 := make([]string, 0, len(dgs))
	for _, d := range dgs {
		if len(d) >= 19 { // "sha256:" + 12
			first12 = append(first12, d[:19])
		} else {
			first12 = append(first12, d)
		}
	}

	check := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", dockerName, "ctr", "-n", ns}, args...)...)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			// treat "no such" as empty
			if !strings.Contains(out.String(), "not found") {
				return fmt.Errorf("%s: %v\n%s", strings.Join(args, " "), err, out.String())
			}
		}
		text := out.String()
		for _, p := range first12 {
			if strings.Contains(text, p) {
				// find full match (best effort)
				for _, full := range dgs {
					if strings.HasPrefix(full, p) {
						set[full] = struct{}{}
					}
				}
			}
		}
		return nil
	}

	if err := check("containers", "ls"); err != nil {
		return nil, err
	}
	if err := check("snapshots", "ls"); err != nil {
		return nil, err
	}
	return set, nil
}

func buildKeepSets(imageTags []string) map[string]string {
	keepByName := make(map[string]string)
	for _, imageTag := range imageTags {
		imageAndTag := strings.Split(imageTag, ":")
		if len(imageAndTag) != 2 {
			continue
		}
		keepByName[imageAndTag[0]] = imageAndTag[1]
	}
	return keepByName
}

func linesTrim(s string) []string {
	all := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(all))
	for _, l := range all {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
