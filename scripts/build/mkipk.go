package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "usage: mkipk OUTPUT debian-binary control.tar.gz data.tar.gz")
		os.Exit(2)
	}
	if err := writeArchive(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "mkipk: %v\n", err)
		os.Exit(1)
	}
}

func writeArchive(output string, inputs []string) error {
	out, err := os.Create(output)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := out.WriteString("!<arch>\n"); err != nil {
		return err
	}
	for _, input := range inputs {
		if err := appendFile(out, input); err != nil {
			return err
		}
	}
	return nil
}

func appendFile(out *os.File, input string) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	name := filepath.Base(input)
	if len(name) > 15 {
		return fmt.Errorf("ar member name %q is too long", name)
	}
	header := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
		name+"/",
		time.Now().Unix(),
		0,
		0,
		0o100644,
		info.Size(),
	)
	if len(header) != 60 {
		return fmt.Errorf("internal ar header length = %d, want 60", len(header))
	}
	if _, err := out.WriteString(header); err != nil {
		return err
	}
	in, err := os.Open(input)
	if err != nil {
		return err
	}
	defer in.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if info.Size()%2 != 0 {
		_, err = out.Write([]byte{'\n'})
		return err
	}
	return nil
}
