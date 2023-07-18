package perf

import (
	"bytes"
	"testing"
)

func TestParseEventStart(t *testing.T) {
	test := func(s string, comm string, pid int, tid int, ok bool) {
		t.Run(s, func(t *testing.T) {
			rComm, rPid, rTid, err := parseEventStart([]byte(s))
			if ok {
				if err != nil {
					t.Errorf("expected no error")
				}
				if comm != string(rComm) {
					t.Errorf("comm %v != %v", comm, rComm)
				}
				if rPid != pid {
					t.Errorf("pid %v != %v", rPid, pid)
				}
				if rTid != tid {
					t.Errorf("tid %v != %v", rTid, tid)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error ")
			}
		})
	}

	test("java 12688 [002] 6544038.708352: cpu-clock:", "java", 12688, 0, true)
	test("V8 WorkerThread 25607 4794564.109216: cycles:", "V8 WorkerThread", 25607, 0, true)
	test("java 24636/25607 [000] 4794564.109216: cycles:", "java", 24636, 25607, true)
	test("java 12688/12764 6544038.708352: cpu-clock:", "java", 12688, 12764, true)
	test("V8 WorkerThread 24636/25607 [000] 94564.109216: cycles:", "V8 WorkerThread", 24636, 25607, true)
	test("", "", 0, 0, false)
	test("\n", "", 0, 0, false)
	test("swapper;start_kernel;rest_init;cpu_idle;default_idle;native_safe_halt 1\n", "", 0, 0, false)
}

func TestParseStackFrame(t *testing.T) {
	test := func(s string, addr, sym, mod string, ok bool) {
		t.Run(s, func(t *testing.T) {
			rAddr, rSym, rMod, err := parseStackFrame([]byte(s))
			if ok {
				if err != nil {
					t.Errorf("expected no error")
				}
				if string(rAddr) != addr {
					t.Errorf("addr %v != %v", rAddr, addr)
				}
				if string(rSym) != sym {
					t.Errorf("mod %v != %v", rSym, sym)
				}
				if string(rMod) != mod {
					t.Errorf("mod %v != %v", rMod, mod)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error<")
			}
		})
	}
	test("        ffffffffb37b3618 __cgroup_account_cputime+0x28 (/lib/modules/5.19.0/build/vmlinux)\n",
		"ffffffffb37b3618", "__cgroup_account_cputime+0x28", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb37105ed update_curr+0x10d (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb37105ed",
		"update_curr+0x10d", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb3712293 dequeue_entity+0x23 (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb3712293",
		"dequeue_entity+0x23", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb3712773 dequeue_task_fair+0xb3 (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb3712773",
		"dequeue_task_fair+0xb3", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb36ff7e2 dequeue_task+0x42 (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb36ff7e2",
		"dequeue_task+0x42", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb4408753 __schedule+0x3e3 (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb4408753",
		"__schedule+0x3e3", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb440982c schedule+0x5c (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb440982c",
		"schedule+0x5c", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb379d3f8 futex_wait_queue+0x78 (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb379d3f8",
		"futex_wait_queue+0x78", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb379daba futex_wait+0x15a (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb379daba",
		"futex_wait+0x15a", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb379a2f8 do_futex+0x138 (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb379a2f8",
		"do_futex+0x138", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb379a7c8 __x64_sys_futex+0x78 (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb379a7c8",
		"__x64_sys_futex+0x78", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb43f916c do_syscall_64+0x5c (/lib/modules/5.19.0/build/vmlinux)\n", "ffffffffb43f916c",
		"do_syscall_64+0x5c", "/lib/modules/5.19.0/build/vmlinux", true)
	test("        ffffffffb460009b entry_SYSCALL_64_after_hwframe+0x63 (/lib/modules/5.19.0/build/vmlinux)\n",
		"ffffffffb460009b", "entry_SYSCALL_64_after_hwframe+0x63", "/lib/modules/5.19.0/build/vmlinux", true)
	test("                   91197 __GI___strncasecmp_l_sse2+0x1547 (/usr/lib/x86_64-linux-gnu/libc.so.6)\n",
		"91197", "__GI___strncasecmp_l_sse2+0x1547", "/usr/lib/x86_64-linux-gnu/libc.so.6", true)
	test("                  3fa48f [unknown] ([unknown])\n", "3fa48f", "[unknown]", "[unknown]", true)
	test("java 12688 [002] 6544038.708352: cpu-clock:", "", "", "", false)
	test("V8 WorkerThread 25607 4794564.109216: cycles:", "", "", "", false)
	test("java 24636/25607 [000] 4794564.109216: cycles:", "", "", "", false)
	test("java 12688/12764 6544038.708352: cpu-clock:", "", "", "", false)
	test("V8 WorkerThread 24636/25607 [000] 94564.109216: cycles:", "", "", "", false)
	test("", "", "", "", false)
	test("\n", "", "", "", false)
}

func TestParseSingleEvent(t *testing.T) {
	event := "perf 617960 [004] 116825.359144:         16   cycles: \n" +
		"        ffffffffb377256a exit_to_user_mode_prepare+0x6a (/lib/modules/5.19.0/build/vmlinux)\n" +
		"        ffffffffb43fe8a6 syscall_exit_to_user_mode+0x26 (/lib/modules/5.19.0/build/vmlinux)\n" +
		"        ffffffffb43f9179 do_syscall_64+0x69 (/lib/modules/5.19.0/build/vmlinux)\n" +
		"        ffffffffb460009b entry_SYSCALL_64_after_hwframe+0x63 (/lib/modules/5.19.0/build/vmlinux)\n" +
		"                  11aaff setsourcefilter+0x18f (/usr/lib/x86_64-linux-gnu/libc.so.6)\n" +
		"                  3364c7 __evlist__enable.constprop.0+0x97 (/home/korniltsev/github/jammy/tools/perf/perf)\n" +
		"                  296253 __cmd_record.constprop.0+0x2683 (/home/korniltsev/github/jammy/tools/perf/perf)\n" +
		"                  297db7 cmd_record+0xbb7 (/home/korniltsev/github/jammy/tools/perf/perf)\n" +
		"                  31ecd0 run_builtin+0x70 (/home/korniltsev/github/jammy/tools/perf/perf)\n" +
		"                  27ae79 main+0x6a9 (/home/korniltsev/github/jammy/tools/perf/perf)\n" +
		"                   29d90 __vstrfmon_l_internal+0x4e0 (/usr/lib/x86_64-linux-gnu/libc.so.6)\n" +
		"\n"
	p := NewScriptParser([]byte(event))
	events, err := p.ParseEvents()
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event")
	}
	stack := events[0]
	expected := [][]byte{
		[]byte("perf"),
		[]byte("__vstrfmon_l_internal+0x4e0"),
		[]byte("main+0x6a9"),
		[]byte("run_builtin+0x70"),
		[]byte("cmd_record+0xbb7"),
		[]byte("__cmd_record.constprop.0+0x2683"),
		[]byte("__evlist__enable.constprop.0+0x97"),
		[]byte("setsourcefilter+0x18f"),
		[]byte("entry_SYSCALL_64_after_hwframe+0x63"),
		[]byte("do_syscall_64+0x69"),
		[]byte("syscall_exit_to_user_mode+0x26"),
		[]byte("exit_to_user_mode_prepare+0x6a"),
	}
	for i := 0; i < len(expected); i++ {
		if !bytes.Equal(expected[i], stack[i]) {
			t.Fatalf("expected %s got %s", string(expected[i]), string(stack[i]))
		}
	}
}

func TestMultipleEvents(t *testing.T) {
	event := "perf 617960 [004] 116825.359144:         16   cycles: \n" +
		"        ffffffffb460009b entry_SYSCALL_64_after_hwframe+0x63 (/lib/modules/5.19.0/build/vmlinux)\n" +
		"                  27ae79 main+0x6a9 (/home/korniltsev/github/jammy/tools/perf/perf)\n" +
		"\n" +
		"perf 617960 [004] 116825.359144:         16   cycles: \n" +
		"        ffffffffb43f9179 do_syscall_64+0x69 (/lib/modules/5.19.0/build/vmlinux)\n" +
		"                  31ecd0 run_builtin+0x70 (/home/korniltsev/github/jammy/tools/perf/perf)\n" +
		"\n"
	p := NewScriptParser([]byte(event))
	events, err := p.ParseEvents()
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events")
	}

	expect := func(expected [][]byte, actual [][]byte) {
		for i := 0; i < len(expected); i++ {
			if !bytes.Equal(expected[i], actual[i]) {
				t.Fatalf("expected %s got %s", string(expected[i]), string(actual[i]))
			}
		}
	}
	expect([][]byte{
		[]byte("perf"),
		[]byte("main+0x6a9"),
		[]byte("entry_SYSCALL_64_after_hwframe+0x63"),
	}, events[0])
	expect([][]byte{
		[]byte("perf"),
		[]byte("run_builtin+0x70"),
		[]byte("do_syscall_64+0x69"),
	}, events[1])
}
