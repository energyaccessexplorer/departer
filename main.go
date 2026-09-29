package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/satori/go.uuid"
	"gitlab.com/noop.nu/srv"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type payload struct {
	IDS   []string `json:"ids"`
	OS    string   `json:"os"`
	Depth int      `json:"depth"`
}

type arrayFlag []string

type H map[string]srv.Handler

func (i *arrayFlag) String() string {
	return strings.Join(*i, ",")
}

func (i *arrayFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// Set at build time (-ldflags -X main.COMMIT_SHA=…); reported by /commit so CI
// (and anyone else) can verify which revision a box is actually running.
var COMMIT_SHA string

var (
	roles      arrayFlag
	pubkeyfile string
	socket     string
	tmpdir     string
	script     string
)

func _check(w http.ResponseWriter, r *http.Request) {
	var i struct {
		Role string `json:"role"`
	}
	srv.JWT_JSON(r, &i)

	io.WriteString(w, i.Role)
}

func _commit(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, COMMIT_SHA)
}

func _build(w http.ResponseWriter, r *http.Request) {
	var p = payload{}

	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := uuid.NewV4().String()

	// The export is built with the requester's own permissions, so hand their
	// token down to the build instead of relying on a shared token in the
	// offroad workspace. It travels in the child's environment, not in argv:
	// /proc/<pid>/cmdline (what ps shows) is world-readable, /proc/<pid>/environ
	// is only readable by the same user. Note the offroad makefile includes its
	// .env after the environment, so a token there would still win.
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	go build(p, id, token)

	io.WriteString(w, fmt.Sprintf(`{ "id": "%s" }`, id))
}

func build(p payload, id string, token string) {
	file, _ := os.Create(tmpdir + "/" + id)
	outfile, _ := os.Create(tmpdir + "/" + id + ".log")
	defer outfile.Close()

	io.WriteString(file, strings.Join(p.IDS, "\n")+"\n")

	cmd := exec.Command(script, tmpdir, id, p.OS)
	if token != "" {
		cmd.Env = append(os.Environ(), "OFFROAD_TOKEN="+token)
	}
	cmd.Stdout = outfile
	cmd.Stderr = outfile

	err := cmd.Run()
	if err != nil {
		fmt.Println(err)
	}

	if werr, ok := err.(*exec.ExitError); ok {
		if s := werr.Error(); s != "0" {
			fmt.Println("Got:", s)
		}
	}
}

func system_check() {
	fmt.Printf("Role claims are not checked (any validly signed token is accepted, as in paver); -role flags are accepted but unused: %s\n", roles)

	_, err := os.Stat(pubkeyfile)
	if os.IsNotExist(err) {
		panic("Public key file does not exist")
	}

	fmt.Printf("Public key: %s\n", pubkeyfile)

	_, err = os.Stat(script)
	if os.IsNotExist(err) {
		panic("THE script does not exist")
	}

	fmt.Printf("Departer script: %s\n", script)

	_, err = os.Stat(tmpdir)
	if os.IsNotExist(err) {
		log.Println(errors.New("Specified temporary directory does not exist. Creating..."))
		os.Mkdir(tmpdir, 0755)
	}

	t, err := os.Open(tmpdir)
	if err != nil {
		log.Fatal(errors.New("Specified temporary directory (still) does not exist!"))
	}
	t.Close()

	fmt.Printf("Temporary directory: %s\n", tmpdir)
}

func parse_flags() {
	flag.Var(&roles, "role", "Roles permitted in the JWT claims")
	flag.StringVar(&pubkeyfile, "pubkey", "", "Public key file to check JWTs")
	flag.StringVar(&socket, "socket", "/tmp/departer-server.sock", "UNIX Socket file")
	flag.StringVar(&tmpdir, "tmpdir", "/tmp/departer", "Temporary directory")
	flag.StringVar(&script, "script", "./departer.sh", "THE script")

	flag.Parse()
}

func main() {
	parse_flags()

	system_check()

	routes := []srv.Route{
		{"/build", []string{"*"}, H{"POST": _build}},
		{"/check", []string{"*"}, H{"GET": _check}},
		{"/commit", nil, H{"GET": _commit}},
	}

	// "*" (see srv.jwt_check) means "any validly signed token": the request must
	// carry a JWT this service can verify, but its `role` claim is not consulted.
	// That matches paver, which gates its routes the same way, and PostgREST,
	// which reads the database role from a claim that does not exist here
	// (jwt-role-claim-key = ".norole"). The -role flags are still accepted so
	// existing DEPARTER_CMD lines keep parsing; they no longer gate anything.

	srv.Run(socket, routes, pubkeyfile)
}
