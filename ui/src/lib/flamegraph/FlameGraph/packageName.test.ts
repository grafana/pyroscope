import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { getPackageName } from './packageName.ts';

// Keep the C++ and Rust cases in their own tables so the intent of the package
// grouping is clear when one of them starts failing.
const cppCases: Array<[name: string, expected: string]> = [
  ['QImageReader::read()', 'QImageReader'],
  ['QCss::Parser::init(QString const&, bool)', 'QCss'],
  ['Window::Window(Options, QWidget*)', 'Window'],
  ['std::vector<int, std::allocator<int>>::push_back(int&&)', 'std'],
  [
    '(anonymous namespace)::Invoke(v8::internal::Isolate*)',
    '(anonymous namespace)',
  ],
  ['v8::internal::(anonymous namespace)::Invoke(v8::internal::Isolate*)', 'v8'],
  ['virtual thunk to Foo::bar()', 'Foo'],
];

const rustCases: Array<[name: string, expected: string]> = [
  ['std::sys::thread::unix::Thread::new::thread_start', 'std'],
  ['<std::sys::thread::unix::Thread>::new::thread_start', 'std'],
  [
    'tokio::runtime::context::runtime::enter_runtime::<tokio::runtime::scheduler::multi_thread::worker::run::{closure#0}, ()>',
    'tokio',
  ],
  ['<tokio::runtime::scheduler::multi_thread::worker::Context>::run', 'tokio'],
  [
    'vibe_agents::transcript::spawn_claude_title_watch::{closure#0}',
    'vibe_agents',
  ],
  ['<F as futures_core::future::TryFuture>::try_poll', 'F'],
  ['<&mut T as core::fmt::Display>::fmt', 'T'],
];

const binaryCases: Array<[name: string, expected: string]> = [
  ['libQt6Core.so.6.11.2+0x30788e', 'libQt6Core.so'],
  ['libc.so.6+0xf65ab', 'libc.so'],
  ['vibe-qt (deleted)+0x1d3834', 'vibe-qt'],
];

const otherLanguages: Array<[name: string, expected: string]> = [
  [
    'github.com/grafana/pyroscope/pkg/foo.Bar',
    'github.com/grafana/pyroscope/pkg/foo.',
  ],
  ['urllib3/response.py:579 - stream', 'urllib3/'],
  ['webrick/server.rb:190 - block (2 levels) in start', 'webrick/'],
  ['./node_modules/foo/index.js:bar:12', 'foo'],
];

describe('getPackageName', () => {
  it('groups C++ symbols by namespace or class', () => {
    for (const [name, expected] of cppCases) {
      assert.equal(getPackageName(name), expected, name);
    }
  });

  it('groups Rust symbols by crate', () => {
    for (const [name, expected] of rustCases) {
      assert.equal(getPackageName(name), expected, name);
    }
  });

  it('groups unsymbolized frames by binary', () => {
    for (const [name, expected] of binaryCases) {
      assert.equal(getPackageName(name), expected, name);
    }
  });

  it('keeps the existing language-specific formats', () => {
    for (const [name, expected] of otherLanguages) {
      assert.equal(getPackageName(name), expected, name);
    }
  });
});
