import React from 'react';
import { render, screen } from '@testing-library/react';
import { FlamegraphRenderer } from './index';

const RawProfile = {
  version: 1,
  flamebearer: {
    names: [
      'total',
      'runtime.mcall',
      'runtime.park_m',
      'runtime.schedule',
      'runtime.resetspinning',
      'runtime.wakep',
      'runtime.startm',
      'runtime.notewakeup',
      'runtime.futexwakeup',
      'runtime.futex',
      'runtime.findrunnable',
      'runtime.write',
      'runtime.write1',
      'runtime.stopm',
      'runtime.mput',
      'runtime.mPark',
      'runtime.notesleep',
      'runtime.futexsleep',
      'runtime.stealWork',
      'runtime.checkTimers',
      'runtime.runtimer',
      'runtime.runOneTimer',
      'time.sendTime',
      'runtime.selectnbsend',
      'runtime.chansend',
      'runtime.adjusttimers',
      'runtime.netpoll',
      'runtime.read',
      'runtime.epollwait',
      'runtime.siftdownTimer',
      'runtime.gosched_m',
      'runtime.unlockWithRank',
      'runtime.unlock2',
      'runtime.lockWithRank',
      'runtime.lock2',
      'runtime.procyield',
      'runtime.goschedImpl',
      'runtime.nanotime',
      'runtime.globrunqget',
      'runtime.(*gQueue).pop',
      'runtime.execute',
      'runtime.casgstatus',
      'runtime.gcBgMarkWorker',
      'runtime.systemstack',
      'runtime.gcBgMarkWorker.func2',
      'runtime.gcDrain',
      'runtime.spanOfUnchecked',
      'runtime.scanobject',
      'runtime.spanOf',
      'runtime.pageIndexOf',
      'runtime.markBits.isMarked',
      'runtime.greyobject',
      'runtime.findObject',
      'runtime.arenaIndex',
      'runtime.(*mspan).divideByElemSize',
      'runtime.(*gcWork).putFast',
      'runtime.heapBitsForAddr',
      'runtime.heapBits.next',
      'runtime.heapBits.bits',
      'runtime.gcFlushBgCredit',
      'runtime.bgsweep',
      'runtime.sweepone',
      'runtime.(*sweepLocked).sweep',
      'runtime.(*gcBitsArena).tryAlloc',
      'runtime.(*sweepLocker).dispose',
      'runtime.(*sweepLocker).blockCompletion',
      'net/http.(*conn).serve',
      'net/http.serverHandler.ServeHTTP',
      'net/http.HandlerFunc.ServeHTTP',
      'github.com/klauspost/compress/gzhttp.NewWrapper.func1.1',
      'github.com/gorilla/mux.(*Router).ServeHTTP',
      'github.com/pyroscope-io/pyroscope/pkg/server.(*Controller).drainMiddleware.func1',
      'github.com/pyroscope-io/pyroscope/pkg/server.ingestHandler.ServeHTTP',
      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.IterateRaw',
      'io.LimitReader',
      'runtime.newobject',
      'runtime.nextFreeFast',
      'io.Copy',
      'io.copyBuffer',
      'bytes.(*Buffer).ReadFrom',
      'io.(*LimitedReader).Read',
      'net/http.(*response).finishRequest',
      'bufio.(*Writer).Flush',
      'net/http.checkConnErrorWriter.Write',
      'net.(*conn).Write',
      'net.(*netFD).Write',
      'syscall.Write',
      'syscall.write',
      'syscall.Syscall',
      'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct.(*Direct).uploadLoop',
      'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct.(*Direct).safeUpload',
      'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct.(*Direct).uploadProfile',
      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Iterate',
      'runtime.growslice',
      'runtime.mallocgc',
      'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots',
      'runtime.selectgo',
      'runtime.gopark',
      'runtime.(*mcache).nextFree',
      'runtime.(*mcache).refill',
      'runtime.(*mcentral).cacheSpan',
      'runtime.mapiterinit',
      'runtime.makemap_small',
      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.(*GoSpy).Snapshot',
      'runtime/pprof.writeHeap',
      'runtime/pprof.writeHeapInternal',
      'runtime/pprof.writeHeapProto',
      'runtime/pprof.(*profileBuilder).pbSample',
      'runtime/pprof.(*profileBuilder).flush',
      'compress/flate.(*Writer).Write',
      'compress/flate.(*compressor).write',
      'compress/flate.(*compressor).encSpeed',
      'compress/flate.(*deflateFast).encode',
      'compress/flate.emitLiteral',
      'runtime/pprof.(*profileBuilder).appendLocsForStack',
      'runtime/pprof.allFrames',
      'runtime.(*Frames).Next',
      'runtime.funcline1',
      'runtime.pcvalue',
      'runtime.step',
      'runtime.GC',
      'runtime.(*spanSet).push',
      'runtime.(*headTailIndex).incTail',
      'runtime.(*mheap).freeSpan',
      'runtime.(*mheap).nextSpanForSweep',
      'runtime.(*spanSet).pop',
      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Profile).Get',
      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.FindFunctionName',
      'runtime.asyncPreempt',
      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.FindLocation',
      'sort.Search',
      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.FindFunction',
      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.FindFunction.func1',
      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.(*GoSpy).Snapshot.func3',
      'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots.func1',
      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Insert',
      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*trieNode).findNodeAt',
      'runtime.(*mcentral).grow',
      'runtime.(*mheap).alloc',
      'runtime.(*mheap).alloc.func1',
      'runtime.(*mheap).allocSpan',
      'runtime.(*mheap).allocMSpanLocked',
      'runtime.(*fixalloc).alloc',
      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.getHeapProfile',
      'github.com/pyroscope-io/pyroscope/pkg/convert.ParsePprof',
      'google.golang.org/protobuf/proto.Unmarshal',
      'google.golang.org/protobuf/proto.UnmarshalOptions.unmarshal',
      'google.golang.org/protobuf/internal/impl.(*MessageInfo).unmarshal',
      'google.golang.org/protobuf/internal/impl.(*MessageInfo).unmarshalPointer',
      'google.golang.org/protobuf/internal/impl.consumeMessageSliceInfo',
      'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).isDueForReset',
      'time.Time.Truncate',
      'github.com/dgraph-io/badger/v2.(*levelsController).runCompactor',
      'github.com/dgraph-io/badger/v2.(*levelsController).pickCompactLevels',
      'sort.Slice',
      'github.com/dgraph-io/badger/v2.(*DB).doWrites.func1',
      'github.com/dgraph-io/badger/v2.(*DB).writeRequests',
      'github.com/dgraph-io/badger/v2.(*valueLog).write',
      'github.com/dgraph-io/badger/v2.(*logFile).encodeEntry',
      'hash/crc32.New',
      'runtime.heapBitsSetType',
      'runtime.newstack',
      'runtime.copystack',
      'runtime.memmove',
    ],
    levels: [
      [0, 236, 0, 0],
      [
        0, 1, 0, 155, 0, 1, 0, 152, 0, 29, 0, 95, 0, 1, 0, 89, 0, 3, 0, 66, 0,
        11, 0, 60, 0, 154, 0, 42, 0, 36, 0, 1,
      ],
      [
        0, 1, 0, 156, 0, 1, 0, 153, 0, 1, 0, 150, 0, 23, 0, 103, 0, 1, 0, 102,
        0, 1, 1, 101, 0, 1, 0, 75, 0, 2, 1, 96, 0, 1, 0, 90, 0, 1, 0, 81, 0, 2,
        0, 67, 0, 2, 2, 65, 0, 3, 3, 64, 0, 6, 3, 61, 0, 154, 0, 43, 0, 19, 0,
        30, 0, 17, 0, 2,
      ],
      [
        0, 1, 0, 157, 0, 1, 1, 154, 0, 1, 1, 151, 0, 1, 0, 143, 0, 12, 0, 126,
        0, 7, 0, 120, 0, 3, 0, 104, 0, 1, 0, 75, 1, 1, 0, 94, 1, 1, 1, 97, 0, 1,
        0, 91, 0, 1, 0, 82, 0, 2, 0, 68, 8, 3, 2, 62, 0, 154, 0, 44, 0, 15, 0,
        36, 0, 2, 0, 33, 0, 2, 0, 31, 0, 17, 0, 3,
      ],
      [
        0, 1, 0, 158, 2, 1, 0, 144, 0, 7, 0, 133, 0, 5, 0, 127, 0, 7, 1, 61, 0,
        3, 0, 105, 0, 1, 1, 94, 1, 1, 0, 98, 2, 1, 0, 92, 0, 1, 0, 83, 0, 2, 0,
        69, 10, 1, 1, 63, 0, 154, 14, 45, 0, 1, 1, 41, 0, 14, 0, 3, 0, 2, 0, 34,
        0, 2, 2, 32, 0, 16, 1, 10, 0, 1, 0, 4,
      ],
      [
        0, 1, 0, 159, 2, 1, 0, 145, 0, 7, 0, 134, 0, 3, 0, 131, 0, 1, 0, 129, 0,
        1, 1, 128, 1, 1, 0, 124, 0, 5, 0, 62, 0, 3, 0, 106, 2, 1, 0, 99, 2, 1,
        0, 93, 0, 1, 0, 84, 0, 2, 0, 70, 25, 1, 1, 59, 0, 7, 7, 58, 0, 5, 5, 57,
        0, 6, 6, 56, 0, 119, 61, 47, 0, 2, 2, 46, 1, 2, 1, 40, 0, 4, 0, 10, 0,
        3, 0, 33, 0, 2, 2, 37, 0, 3, 0, 31, 0, 2, 2, 35, 3, 1, 0, 19, 0, 5, 0,
        26, 0, 2, 0, 18, 0, 6, 0, 13, 0, 1, 0, 11, 0, 1, 0, 5,
      ],
      [
        0, 1, 0, 75, 2, 1, 0, 146, 0, 7, 0, 135, 0, 3, 2, 130, 0, 1, 1, 130, 2,
        1, 1, 125, 0, 1, 0, 123, 0, 4, 3, 121, 0, 1, 0, 114, 0, 2, 0, 107, 2, 1,
        1, 100, 2, 1, 1, 94, 0, 1, 0, 85, 0, 2, 0, 68, 105, 2, 2, 55, 0, 3, 3,
        54, 0, 1, 1, 53, 0, 17, 17, 52, 0, 3, 3, 51, 0, 5, 5, 50, 0, 11, 11, 49,
        0, 16, 16, 48, 4, 1, 1, 41, 0, 1, 1, 39, 0, 1, 1, 38, 0, 1, 1, 37, 0, 1,
        0, 26, 0, 3, 0, 34, 2, 3, 3, 32, 5, 1, 0, 20, 0, 4, 4, 28, 0, 1, 1, 27,
        0, 2, 0, 19, 0, 5, 0, 15, 0, 1, 1, 14, 0, 1, 1, 12, 0, 1, 0, 6,
      ],
      [
        0, 1, 0, 94, 2, 1, 0, 147, 0, 7, 6, 136, 2, 1, 1, 132, 4, 1, 0, 43, 3,
        1, 1, 122, 0, 1, 0, 115, 0, 2, 0, 108, 6, 1, 0, 86, 0, 2, 0, 71, 171, 1,
        1, 28, 0, 3, 3, 35, 10, 1, 0, 21, 5, 1, 1, 25, 0, 1, 0, 20, 0, 5, 0, 16,
        2, 1, 0, 7,
      ],
      [
        0, 1, 0, 160, 2, 1, 0, 148, 6, 1, 0, 94, 7, 1, 0, 33, 4, 1, 0, 116, 0,
        2, 0, 109, 6, 1, 0, 87, 0, 2, 0, 72, 185, 1, 1, 29, 6, 1, 0, 21, 0, 5,
        0, 17, 2, 1, 0, 8,
      ],
      [
        0, 1, 0, 161, 2, 1, 0, 149, 6, 1, 0, 98, 7, 1, 1, 34, 4, 1, 0, 117, 0,
        2, 0, 110, 6, 1, 1, 88, 0, 2, 0, 73, 192, 1, 0, 22, 0, 5, 5, 9, 2, 1, 1,
        9,
      ],
      [
        0, 1, 0, 162, 2, 1, 1, 148, 6, 1, 0, 99, 12, 1, 0, 118, 0, 2, 0, 111, 7,
        1, 0, 77, 0, 1, 0, 74, 192, 1, 0, 23,
      ],
      [
        0, 1, 1, 163, 9, 1, 0, 100, 12, 1, 1, 119, 0, 2, 1, 112, 7, 1, 0, 78, 0,
        1, 0, 75, 192, 1, 1, 24,
      ],
      [10, 1, 0, 137, 14, 1, 1, 113, 7, 1, 0, 79, 0, 1, 1, 76],
      [10, 1, 0, 138, 22, 1, 1, 80],
      [10, 1, 0, 43],
      [10, 1, 0, 139],
      [10, 1, 0, 140],
      [10, 1, 0, 141],
      [10, 1, 1, 142],
    ],
    numTicks: 236,
    maxSelf: 61,
  },
  timeline: {
    startTime: 1646760440,
    samples: [237],
    durationDelta: 10,
    watermarks: {},
  },
  metadata: {
    format: 'single' as const,
    spyName: 'gospy' as const,
    sampleRate: 100,
    units: 'samples' as const,
    name: 'pyroscope.server.cpu 2022-03-08T17:27:23Z',
    appName: 'pyroscope.server.cpu',
    startTime: 1646760443,
    endTime: 1646760446,
    query: 'pyroscope.server.cpu{}',
    maxNodes: 1024,
  },
};

const SimpleTree = {
  topLevel: 0,
  rangeMin: 0,
  format: 'single' as const,
  numTicks: 988,
  sampleRate: 100,
  names: [
    'total',
    'runtime.main',
    'main.slowFunction',
    'main.work',
    'main.main',
    'main.fastFunction',
  ],
  levels: [
    [0, 988, 0, 0],
    [0, 988, 0, 1],
    [0, 214, 0, 5, 214, 3, 2, 4, 217, 771, 0, 2],
    [0, 214, 214, 3, 216, 1, 1, 5, 217, 771, 771, 3],
  ],

  rangeMax: 1,
  units: 'samples',
  fitMode: 'HEAD',

  spyName: 'gospy',
};

// describe.skip('Pyroscope Library', () => {
//  it('should not be possible to override the pyroscope logo using props', () => {
//    render(
//      <FlamegraphRenderer
//        flamebearer={SimpleTree}
//        display="flamegraph"
//        viewType="single"
//        showPyroscopeLogo={false}
//      />
//    );
//
//    expect(
//      screen.getByRole('link', { name: /pyroscope/i })
//    ).toBeInTheDocument();
//  });
// });
//
//
// TODO a test saying going over rendering an empty flamegraph
describe.only('positions', () => {
  beforeAll(() => {
    window.HTMLElement.prototype.getBoundingClientRect = function () {
      return {
        x: 0,
        y: 0,
        bottom: 0,
        right: 0,
        toJSON: () => {},
        height: 0,
        top: 0,
        left: 0,
        width: 900,
      };
    };
  });

  describe('Allow changing visualization mode', () => {
    it('should allow changing view when "onlyDisplay" is not set', () => {
      const { getByTestId, queryByRole } = render(
        <FlamegraphRenderer profile={RawProfile} />
      );

      expect(getByTestId('table-ui')).toBeInTheDocument();
      expect(getByTestId('flamegraph-view')).toBeInTheDocument();
      expect(getByTestId('flamegraph')).toBeInTheDocument();
      expect(getByTestId('both')).toBeInTheDocument();
      expect(getByTestId('table')).toBeInTheDocument();
    });

    it('should restrict changing view when "onlyDisplay" is set', () => {
      const { getByTestId, queryByRole } = render(
        <FlamegraphRenderer profile={RawProfile} onlyDisplay="both" />
      );

      expect(getByTestId('table-ui')).toBeInTheDocument();
      expect(getByTestId('flamegraph-view')).toBeInTheDocument();
      expect(queryByRole('combobox', { name: 'view' })).not.toBeInTheDocument();
    });
  });

  it('should display only the flamegraph when specified', () => {
    const { getByTestId, queryByTestId } = render(
      <FlamegraphRenderer profile={RawProfile} onlyDisplay="flamegraph" />
    );

    expect(queryByTestId('table-ui')).not.toBeInTheDocument();
    expect(getByTestId('flamegraph-view')).toBeInTheDocument();
  });

  it('should display only the table when specified', () => {
    const { getByTestId, queryByTestId } = render(
      <FlamegraphRenderer profile={RawProfile} onlyDisplay="table" />
    );

    expect(getByTestId('table-ui')).toBeInTheDocument();
    expect(queryByTestId('flamegraph-view')).not.toBeInTheDocument();
  });
});

it('should work', () => {
  render(<FlamegraphRenderer flamebearer={SimpleTree as any} />);
});

it('should work with raw profile from /render endpoint', () => {
  render(<FlamegraphRenderer profile={RawProfile} />);
});
