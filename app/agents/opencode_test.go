package agents

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func seedOpenCodeDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		"create table session (id text primary key, project_id text, parent_id text, slug text, directory text, title text, version text, time_created integer, time_updated integer)",
		"create table message (id text primary key, session_id text, time_created integer, time_updated integer, data text)",
		"create table part (id text primary key, message_id text, session_id text, time_created integer, time_updated integer, data text)",
		"create table todo (session_id text, content text, status text, priority text, position integer, time_created integer, time_updated integer)",
		`insert into session values ('s_old','p',null,'x','/home/u/proj','Old','1',1,100)`,
		`insert into session values ('s_new','p',null,'y','/home/u/proj','New','1',2,200)`,
		`insert into session values ('s_other','p',null,'z','/elsewhere','Other','1',3,300)`,
		`insert into message values ('m1','s_new',1000,1000,'{"role":"user","time":{"created":1000}}')`,
		`insert into part values ('p1','m1','s_new',1001,1001,'{"type":"text","text":"please fix the build"}')`,
		`insert into message values ('m2','s_new',2000,2000,'{"role":"assistant","time":{"created":2000,"completed":2500}}')`,
		`insert into part values ('p2','m2','s_new',2001,2001,'{"type":"text","text":"Running the build:"}')`,
		`insert into part values ('p3','m2','s_new',2002,2002,'{"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"go build ./...","description":"Build"},"metadata":{"exit":1}}}')`,
		`insert into part values ('p4','m2','s_new',2003,2003,'{"type":"tool","tool":"edit","state":{"status":"completed","input":{"filePath":"/home/u/proj/main.go"}}}')`,
		`insert into part values ('p5','m2','s_new',2004,2004,'{"type":"patch","files":["/home/u/proj/go.mod"]}')`,
		`insert into part values ('p6','m2','s_new',2005,2005,'{"type":"text","text":"Fixed. ` + "```go\\nfunc main() {}\\n```" + `"}')`,
		`insert into todo values ('s_new','Fix build','completed','high',0,1,1)`,
		`insert into todo values ('s_new','Run tests','in_progress','medium',1,1,1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return path
}

func TestReadOpenCodeTranscript(t *testing.T) {
	path := seedOpenCodeDB(t)
	tr, err := ReadOpenCodeTranscript(path, "/home/u/proj", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if tr.Cwd != "/home/u/proj" || tr.Candidates != 2 || tr.SessionFile != path+"#s_new" {
		t.Fatalf("wrong session: %+v", tr)
	}
	if len(tr.Messages) != 2 || tr.Messages[0].Role != "user" || tr.Messages[1].Role != "assistant" {
		t.Fatalf("messages: %+v", tr.Messages)
	}
	if tr.Messages[1].Text != "Running the build:\n\nFixed. ```go\nfunc main() {}\n```" {
		t.Fatalf("assistant text: %q", tr.Messages[1].Text)
	}
	if len(tr.Commands) != 1 || tr.Commands[0].Command != "go build ./..." || !tr.Commands[0].Failed || tr.Commands[0].ExitCode != 1 {
		t.Fatalf("commands: %+v", tr.Commands)
	}
	if len(tr.Files) != 2 || tr.Files[0].Path != "/home/u/proj/go.mod" && tr.Files[1].Path != "/home/u/proj/go.mod" {
		t.Fatalf("files: %+v", tr.Files)
	}
	if len(tr.Tasks) != 2 || tr.Tasks[1].Status != "in_progress" {
		t.Fatalf("tasks: %+v", tr.Tasks)
	}

	// Unknown cwd → newest session overall (the /elsewhere one).
	tr, err = ReadOpenCodeTranscript(path, "", 0)
	if err != nil || tr.Cwd != "/elsewhere" {
		t.Fatalf("newest session fallback: %+v (%v)", tr, err)
	}
	if _, err := ReadOpenCodeTranscript(path, "/nope", 0); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	st, err := OpenCodeState(path, "/home/u/proj")
	if err != nil || st.State != StateIdle || st.LastText == "" {
		t.Fatalf("state: %+v (%v)", st, err)
	}
	if OpenCodeDBStamp(path) == "" {
		t.Fatalf("stamp must reflect the db file")
	}
}
