import React from 'react';
// import { SimpleSingle as TestData } from '@utils/testData';
import { render as testRender, screen } from '@testing-library/react';
import { Profile } from '@pyroscope/legacy/models';
import 'web-streams-polyfill';
import ExportData, { getFilename } from './ExportData';
import { BrowserRouter } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import { setStore } from '@pyroscope/services/storage';
import { continuousReducer } from '../redux/reducers/continuous';

function createStore(preloadedState: any) {
  const store = configureStore({
    reducer: {
      continuous: continuousReducer,
    },
    preloadedState,
  });
  setStore(store);
  return store;
}

function render(component: any) {
  const store = createStore({
    continuous: {
      from: 'now-1h',
      until: 'now',
      leftFrom: 'now-1h',
      leftUntil: 'now-30m',
      rightFrom: 'now-30m',
      rightUntil: 'now',
      query: 'simple.golang.app.cpu{}',
    },
  });

  return testRender(<Provider store={store}>{component}</Provider>, {
    wrapper: BrowserRouter as any,
  });
}

describe('ExportData', () => {
  it.skip('fails if theres not a single export mode -- Since we force some exports, this never happens', () => {
    // ignore console.error since jsdom will complain
    jest.spyOn(global.console, 'error').mockImplementation(() => jest.fn());

    expect(() => render(<ExportData flamebearer={{} as any} />)).toThrow();

    jest.restoreAllMocks();
  });

  // For these tests, since the code actually navigates to a url
  // We took the easy route and just test for the button presence
  describe('assert button presence', () => {
    it('supports a download JSON button', () => {
      render(<ExportData exportJSON flamebearer={TestData} />);
      screen.getByRole('button', { name: /json/i });
    });

    it('supports a download pprof button', () => {
      render(<ExportData exportPprof flamebearer={TestData} />);
      screen.getByRole('button', { name: /pprof/i });
    });

    it('supports a download png button', () => {
      render(<ExportData exportPNG flamebearer={TestData} />);
      screen.getByRole('button', { name: /png/i });
    });

    it.skip('supports a download html button -- no -- exportHTML is explicitly set to false', () => {
      render(<ExportData exportHTML flamebearer={TestData} />);
      screen.getByRole('button', { name: /html/i });
    });

    describe('the "flamegraph.com" export button', () => {
      it('is enabled by default"', () => {
        render(<ExportData flamebearer={TestData} />);
        screen.getByRole('button', { name: /flamegraph\.com/i });
      });

      it('can be enabled"', () => {
        render(<ExportData exportFlamegraphDotCom flamebearer={TestData} />);
        screen.getByRole('button', { name: /flamegraph\.com/i });
      });

      it('can be disabled"', () => {
        render(
          <ExportData exportFlamegraphDotCom={false} flamebearer={TestData} />
        );
        expect(
          screen.queryByRole('button', { name: /flamegraph\.com/i })
        ).not.toBeInTheDocument();
      });
    });
  });

  describe('filename', () => {
    it('generates a fullname', () => {
      expect(
        getFilename(
          'pyroscope.server.alloc_objects',
          TestData.metadata.startTime,
          TestData.metadata.endTime
        )
      ).toBe(
        'pyroscope.server.alloc_objects_2022-03-09_2025-to-2022-03-09_2025'
      );
    });

    it('uses the appname if its the only thing available', () => {
      expect(getFilename('pyroscope.server.alloc_objects')).toBe(
        'pyroscope.server.alloc_objects'
      );
    });

    it('uses the date if its the only thing available', () => {
      expect(
        getFilename(
          undefined,
          TestData.metadata.startTime,
          TestData.metadata.endTime
        )
      ).toBe('flamegraph_2022-03-09_2025-to-2022-03-09_2025');
    });

    it('uses a generic name if nothing is available ', () => {
      expect(getFilename()).toBe('flamegraph');
    });
  });
});

// const TestData = {
//  version: 1,
//  flamebearer: {
//    names: [
//      'total',
//      'runtime/pprof.profileWriter',
//      'runtime/pprof.newProfileBuilder',
//      'runtime/pprof.(*profileBuilder).readMapping',
//      'runtime/pprof.parseProcSelfMaps',
//      'os.ReadFile',
//      'runtime/pprof.(*profileBuilder).build',
//      'runtime/pprof.(*protobuf).varint',
//      'runtime/pprof.(*protobuf).string',
//      'runtime/pprof.(*profileBuilder).appendLocsForStack',
//      'runtime/pprof.allFrames',
//      'runtime/pprof.(*profileBuilder).stringIndex',
//      'runtime/pprof.(*profileBuilder).emitLocation',
//      'runtime/pprof.(*profileBuilder).pbLine',
//      'runtime/pprof.(*profileBuilder).flush',
//      'compress/gzip.(*Writer).Write',
//      'compress/flate.NewWriter',
//      'compress/flate.(*compressor).init',
//      'runtime/pprof.(*pcDeck).tryAdd',
//      'compress/flate.newDeflateFast',
//      'compress/flate.(*Writer).Close',
//      'compress/flate.(*compressor).close',
//      'compress/flate.(*compressor).encSpeed',
//      'compress/flate.(*huffmanBitWriter).writeBlockDynamic',
//      'compress/flate.(*huffmanBitWriter).indexTokens',
//      'compress/flate.(*huffmanEncoder).generate',
//      'runtime/pprof.(*profileBuilder).addCPUData',
//      'runtime/pprof.(*profMap).lookup',
//      'net/http.HandlerFunc.ServeHTTP',
//      'github.com/slok/go-http-metrics/middleware/std.Handler.func1',
//      'github.com/slok/go-http-metrics/middleware.Middleware.Measure',
//      'github.com/slok/go-http-metrics/middleware/std.Handler.func1.1',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.(*Storage).Get',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.(*Storage).GetContext',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*Segment).GetContext',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*streeNode).get',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*Segment).GetContext.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.(*Storage).GetContext.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache.(*Cache).Lookup',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache.(*Cache).get',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache/lfu.(*Cache).GetOrSet',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache.(*Cache).get.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.treeCodec.Deserialize',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.Deserialize',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/dict.(*Dict).GetValue',
//      'bytes.NewReader',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache.(*Cache).GetOrCreate',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache/lfu.(*Cache).increment',
//      'net/http.(*conn).serve',
//      'net/http.serverHandler.ServeHTTP',
//      'github.com/klauspost/compress/gzhttp.NewWrapper.func1.1',
//      'github.com/klauspost/compress/gzhttp.NewWrapper.func1.1.1',
//      'github.com/klauspost/compress/gzhttp.(*GzipResponseWriter).Close',
//      'github.com/klauspost/compress/gzhttp/writer/gzkp.(*pooledWriter).Close',
//      'github.com/klauspost/compress/flate.(*Writer).Close',
//      'github.com/klauspost/compress/flate.(*compressor).close',
//      'github.com/klauspost/compress/flate.(*compressor).storeFast',
//      'github.com/klauspost/compress/flate.(*fastGen).addBlock',
//      'github.com/gorilla/mux.(*Router).ServeHTTP',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Tree).Merge',
//      'github.com/pyroscope-io/pyroscope/pkg/server.(*Controller).renderHandler',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer.NewProfile',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Tree).FlamebearerStruct',
//      'github.com/pyroscope-io/pyroscope/pkg/server.(*Controller).writeResponseJSON',
//      'encoding/json.(*Encoder).Encode',
//      'github.com/slok/go-http-metrics/middleware/std.(*responseWriterInterceptor).Write',
//      'github.com/klauspost/compress/gzhttp.(*GzipResponseWriter).Write',
//      'github.com/klauspost/compress/gzhttp.(*GzipResponseWriter).startGzip',
//      'github.com/klauspost/compress/gzip.(*Writer).Write',
//      'github.com/klauspost/compress/flate.NewWriter',
//      'github.com/klauspost/compress/flate.newFastEnc',
//      'net/http.(*fileHandler).ServeHTTP',
//      'net/http.serveFile',
//      'net/http.serveContent',
//      'io.Copy',
//      'io.copyBuffer',
//      'github.com/klauspost/compress/flate.(*Writer).Write',
//      'github.com/klauspost/compress/flate.(*compressor).write',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*treeNode).clone',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*Segment).Put',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*streeNode).put',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*Segment).Put.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.(*Storage).Put.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Tree).Clone',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache.New.func2',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/cache.(*Cache).saveToDisk',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.treeCodec.Serialize',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Tree).SerializeTruncate',
//      'github.com/valyala/bytebufferpool.(*ByteBuffer).Write',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/cappedarr.New',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Tree).minValue',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Tree).iterateWithTotal',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/dict.(*Dict).Put',
//      'github.com/pyroscope-io/pyroscope/pkg/util/varint.NewWriter',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/dict.(*trieNode).findNodeAt',
//      'github.com/pyroscope-io/pyroscope/pkg/util/varint.Writer.Write',
//      'bytes.(*Buffer).Write',
//      'bytes.(*Buffer).grow',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.segmentCodec.Serialize',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*Segment).Serialize',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*Segment).serialize',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.dictionaryCodec.Serialize',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/dict.(*Dict).Serialize',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct.(*Direct).uploadLoop',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct.(*Direct).safeUpload',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct.(*Direct).uploadProfile',
//      'github.com/sirupsen/logrus.(*Entry).Debug',
//      'github.com/sirupsen/logrus.(*Entry).Log',
//      'github.com/sirupsen/logrus.Entry.log',
//      'github.com/sirupsen/logrus.(*Entry).write',
//      'github.com/sirupsen/logrus.(*TextFormatter).Format',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Iterate',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Tree).Insert',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.ParseKey',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*parser).nameParserCase',
//      'github.com/pyroscope-io/pyroscope/pkg/storage.(*Storage).Put',
//      'github.com/sirupsen/logrus.(*Logger).WithFields',
//      'github.com/sirupsen/logrus.(*Logger).newEntry',
//      'sync.(*Pool).Get',
//      'sync.(*Pool).pin',
//      'sync.(*Pool).pinSlow',
//      'github.com/sirupsen/logrus.(*Entry).WithFields',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.newNode',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/segment.(*Key).TreeKey',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/sortedmap.New',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/labels.(*Labels).Put',
//      'github.com/dgraph-io/badger/v2.(*DB).Update',
//      'github.com/dgraph-io/badger/v2.(*Txn).Commit',
//      'github.com/dgraph-io/badger/v2.(*Txn).commitAndSend',
//      'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.(*GoSpy).Snapshot',
//      'runtime/pprof.writeHeap',
//      'runtime/pprof.writeHeapInternal',
//      'runtime/pprof.writeHeapProto',
//      'runtime/pprof.(*profileBuilder).pbSample',
//      'github.com/valyala/bytebufferpool.(*ByteBuffer).WriteString',
//      'github.com/pyroscope-io/pyroscope/pkg/storage/tree.(*Profile).Get',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.(*GoSpy).Snapshot.func3',
//      'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Insert',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.newTrieNode',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*trieNode).findNodeAt',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*trieNode).insert',
//      'github.com/pyroscope-io/pyroscope/pkg/agent.mergeTagsWithAppName',
//      'github.com/pyroscope-io/pyroscope/pkg/convert.ParsePprof',
//      'io.ReadAll',
//      'google.golang.org/protobuf/proto.Unmarshal',
//      'google.golang.org/protobuf/proto.UnmarshalOptions.unmarshal',
//      'google.golang.org/protobuf/internal/impl.(*MessageInfo).unmarshal',
//      'google.golang.org/protobuf/internal/impl.(*MessageInfo).unmarshalPointer',
//      'google.golang.org/protobuf/internal/impl.consumeMessageSliceInfo',
//      'reflect.New',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.getHeapProfile',
//      'google.golang.org/protobuf/internal/impl.pointer.AppendPointerSlice',
//      'google.golang.org/protobuf/internal/impl.consumeStringSliceValidateUTF8',
//      'google.golang.org/protobuf/internal/impl.consumeUint64Slice',
//      'google.golang.org/protobuf/internal/impl.consumeInt64Slice',
//      'compress/gzip.NewReader',
//      'compress/gzip.(*Reader).Reset',
//      'compress/gzip.(*Reader).readHeader',
//      'compress/flate.(*dictDecoder).init',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.(*GoSpy).Snapshot.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/agent/gospy.startCPUProfile',
//      'runtime/pprof.StartCPUProfile',
//      'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).reset',
//      'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).uploadTries',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Diff',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Diff.func1',
//      'github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*trieNode).clone',
//      'github.com/dgraph-io/badger/v2.(*DB).updateSize',
//      'github.com/dgraph-io/badger/v2.(*DB).calculateSize',
//      'github.com/dgraph-io/badger/v2.(*DB).calculateSize.func2',
//      'path/filepath.Walk',
//      'path/filepath.walk',
//      'path/filepath.readDirNames',
//      'os.(*File).Readdirnames',
//      'os.(*File).readdir',
//      'github.com/dgraph-io/badger/v2.(*DB).doWrites.func1',
//      'github.com/dgraph-io/badger/v2.(*DB).writeRequests',
//      'github.com/dgraph-io/badger/v2.(*valueLog).write',
//      'github.com/dgraph-io/badger/v2.(*logFile).encodeEntry',
//      'bytes.makeSlice',
//    ],
//    levels: [
//      [0, 4147083, 0, 0],
//      [
//        0, 344, 0, 177, 0, 64, 0, 169, 0, 8193, 0, 58, 0, 2315453, 415074, 129,
//        0, 839135, 0, 103, 0, 861368, 0, 84, 0, 8193, 0, 79, 0, 8193, 0, 78, 0,
//        15438, 0, 48, 0, 17477, 0, 28, 0, 73225, 0, 1,
//      ],
//      [
//        0, 344, 0, 178, 0, 64, 0, 170, 0, 8193, 0, 28, 415074, 345011, 0, 164,
//        0, 1555368, 0, 130, 0, 839135, 0, 104, 0, 861368, 0, 85, 0, 8193, 0, 80,
//        0, 8193, 0, 78, 0, 15438, 0, 49, 0, 17477, 0, 28, 0, 65, 0, 26, 0, 7441,
//        0, 6, 0, 65719, 0, 2,
//      ],
//      [
//        0, 344, 0, 179, 0, 64, 0, 171, 0, 8193, 0, 28, 415074, 345011, 0, 165,
//        0, 778, 745, 157, 0, 46, 0, 161, 0, 686849, 0, 152, 0, 10222, 0, 144, 0,
//        578792, 0, 136, 0, 2731, 2731, 135, 0, 275950, 0, 131, 0, 839135, 4097,
//        105, 0, 26494, 0, 101, 0, 47, 0, 98, 0, 834827, 0, 86, 0, 8193, 0, 81,
//        0, 8193, 0, 78, 0, 15438, 0, 28, 0, 17477, 0, 29, 0, 65, 65, 27, 0, 228,
//        0, 20, 0, 134, 0, 15, 0, 6809, 0, 9, 0, 98, 98, 8, 0, 172, 172, 7, 0,
//        65719, 0, 3,
//      ],
//      [
//        0, 344, 0, 180, 0, 64, 0, 172, 0, 8193, 0, 29, 415074, 345011, 88237,
//        166, 745, 33, 0, 158, 0, 46, 0, 162, 0, 33, 0, 157, 0, 686816, 0, 144,
//        0, 10144, 0, 146, 0, 78, 78, 145, 0, 578792, 0, 137, 2731, 275950, 30,
//        132, 4097, 278195, 0, 115, 0, 32768, 0, 113, 0, 518613, 507690, 111, 0,
//        5462, 0, 106, 0, 26494, 26494, 102, 0, 47, 0, 99, 0, 834827, 105818, 87,
//        0, 8193, 0, 82, 0, 8193, 0, 78, 0, 15438, 0, 50, 0, 17477, 0, 30, 65,
//        228, 0, 21, 0, 134, 29, 16, 0, 3277, 3277, 18, 0, 591, 293, 12, 0, 192,
//        192, 11, 0, 228, 228, 7, 0, 2521, 2521, 10, 270, 182, 182, 5, 0, 65537,
//        65537, 4,
//      ],
//      [
//        0, 344, 0, 96, 0, 64, 0, 173, 0, 8193, 0, 30, 503311, 90117, 0, 168, 0,
//        166657, 0, 141, 745, 33, 0, 159, 0, 46, 46, 163, 0, 33, 0, 158, 0,
//        686346, 0, 146, 0, 470, 470, 145, 0, 10144, 0, 147, 78, 578792, 0, 138,
//        2761, 275920, 24578, 133, 4097, 5462, 0, 125, 0, 255050, 0, 79, 0,
//        17683, 0, 116, 0, 32768, 32768, 114, 507690, 10923, 10923, 112, 0, 5462,
//        0, 107, 26494, 47, 0, 100, 105818, 593133, 396514, 92, 0, 135623, 0, 90,
//        0, 227, 227, 89, 0, 26, 26, 88, 0, 8193, 0, 83, 0, 8193, 0, 78, 0,
//        15436, 0, 58, 0, 2, 0, 51, 0, 17477, 0, 31, 65, 228, 0, 22, 29, 58, 58,
//        17, 0, 47, 47, 19, 3570, 5, 0, 14, 0, 293, 0, 13,
//      ],
//      [
//        0, 344, 0, 97, 0, 64, 0, 174, 0, 8193, 0, 31, 503311, 90117, 0, 168, 0,
//        166657, 166657, 167, 745, 33, 33, 160, 46, 33, 0, 159, 0, 686346, 0,
//        147, 470, 10144, 0, 148, 78, 23667, 0, 143, 0, 555125, 284775, 139,
//        27339, 250440, 0, 9, 0, 271, 0, 6, 0, 43, 0, 134, 0, 588, 0, 2, 4097,
//        5462, 0, 126, 0, 255050, 0, 80, 0, 17426, 17426, 121, 0, 257, 0, 117,
//        551381, 5462, 0, 108, 26494, 47, 0, 100, 502332, 163850, 0, 94, 0,
//        32769, 32769, 93, 0, 135623, 135623, 91, 253, 8193, 0, 78, 0, 8193, 0,
//        78, 0, 5, 0, 71, 0, 15431, 0, 28, 0, 2, 0, 52, 0, 17477, 0, 28, 65, 228,
//        0, 23, 3704, 5, 0, 15, 0, 293, 293, 7,
//      ],
//      [
//        0, 344, 344, 181, 0, 64, 0, 175, 0, 8193, 0, 28, 503311, 90117, 0, 168,
//        167481, 33, 33, 160, 0, 686346, 0, 148, 470, 10144, 0, 149, 78, 1821, 0,
//        113, 0, 21846, 21846, 124, 284775, 122885, 0, 141, 0, 147465, 147465,
//        140, 27339, 820, 201, 12, 0, 147, 147, 11, 0, 410, 410, 7, 0, 249063,
//        249063, 10, 0, 29, 29, 11, 0, 233, 233, 8, 0, 9, 9, 7, 0, 43, 0, 14, 0,
//        588, 0, 3, 4097, 5462, 0, 127, 0, 255050, 0, 81, 17426, 257, 0, 118,
//        551381, 5462, 0, 109, 26494, 47, 0, 100, 502332, 163850, 0, 95, 168645,
//        8193, 0, 78, 0, 8193, 0, 78, 0, 5, 0, 72, 0, 15431, 0, 28, 0, 2, 0, 53,
//        0, 17477, 0, 28, 65, 228, 0, 24, 3704, 5, 3, 16,
//      ],
//      [
//        344, 64, 64, 176, 0, 8193, 0, 28, 503311, 90117, 0, 168, 167514, 686346,
//        0, 149, 470, 10144, 0, 150, 78, 1821, 1821, 114, 306621, 122885, 122885,
//        142, 175005, 107, 0, 14, 0, 512, 512, 7, 249891, 43, 0, 15, 0, 588, 588,
//        5, 4097, 5462, 5462, 128, 0, 255050, 0, 82, 17426, 257, 0, 119, 551381,
//        5462, 5462, 110, 26494, 47, 0, 100, 502332, 163850, 0, 96, 168645, 8193,
//        0, 78, 0, 8193, 0, 78, 0, 5, 0, 73, 0, 15431, 0, 29, 0, 2, 0, 54, 0,
//        17477, 0, 28, 65, 228, 228, 25, 3707, 2, 2, 17,
//      ],
//      [
//        408, 8193, 0, 28, 503311, 90117, 0, 168, 167514, 658478, 0, 150, 0,
//        27309, 27309, 154, 0, 559, 559, 153, 470, 10144, 10144, 151, 606410,
//        107, 0, 15, 250403, 43, 3, 16, 10147, 10923, 0, 123, 0, 196611, 0, 83,
//        0, 47516, 39323, 59, 17426, 257, 257, 120, 583337, 47, 0, 100, 502332,
//        163850, 163850, 97, 168645, 8193, 0, 78, 0, 8193, 0, 78, 0, 5, 0, 74, 0,
//        15431, 0, 30, 0, 2, 0, 55, 0, 17477, 0, 32,
//      ],
//      [
//        408, 8193, 0, 32, 503311, 90117, 0, 168, 167514, 510384, 0, 149, 0,
//        148094, 148094, 151, 644892, 107, 25, 16, 250406, 14, 14, 17, 0, 26, 26,
//        19, 10147, 10923, 10923, 124, 0, 196611, 0, 78, 39323, 8193, 8193, 122,
//        601020, 47, 0, 100, 834827, 8193, 0, 78, 0, 8193, 0, 78, 0, 5, 0, 75, 0,
//        15431, 0, 31, 0, 2, 0, 56, 0, 17477, 0, 33,
//      ],
//      [
//        408, 8193, 0, 33, 503311, 90117, 0, 168, 167514, 163843, 163843, 156, 0,
//        96672, 0, 150, 0, 184332, 184332, 155, 0, 65537, 65537, 153, 793011, 40,
//        40, 17, 0, 42, 42, 19, 271516, 196611, 0, 78, 648536, 15, 0, 100, 0, 32,
//        0, 95, 834827, 8193, 0, 78, 0, 8193, 0, 78, 0, 5, 0, 66, 0, 15431, 0,
//        28, 0, 2, 2, 57, 0, 17477, 0, 34,
//      ],
//      [
//        408, 8193, 0, 34, 503311, 90117, 0, 168, 331357, 96672, 96672, 151,
//        1314478, 196611, 16385, 78, 648536, 15, 0, 95, 0, 32, 32, 88, 834827,
//        8193, 0, 78, 0, 8193, 0, 78, 0, 2, 0, 76, 0, 3, 0, 67, 0, 15431, 0, 28,
//        2, 17477, 0, 35,
//      ],
//      [
//        408, 8193, 0, 35, 503311, 90117, 0, 168, 1758892, 180226, 65537, 78,
//        648536, 15, 15, 88, 834859, 8193, 0, 78, 0, 8193, 0, 78, 0, 2, 0, 77, 0,
//        3, 0, 68, 0, 15431, 0, 28, 2, 17477, 0, 35,
//      ],
//      [
//        408, 8193, 0, 35, 503311, 90117, 0, 168, 1824429, 114689, 8192, 78,
//        1483410, 8193, 0, 78, 0, 8193, 0, 78, 0, 2, 0, 56, 0, 3, 3, 69, 0, 2323,
//        0, 60, 0, 13108, 0, 32, 2, 17477, 0, 35,
//      ],
//      [
//        408, 8193, 0, 35, 503311, 90117, 0, 168, 1832621, 106497, 0, 78,
//        1483410, 8193, 0, 78, 0, 8193, 0, 78, 0, 2, 2, 57, 3, 10, 0, 63, 0,
//        2313, 0, 61, 0, 13108, 0, 33, 2, 17477, 0, 35,
//      ],
//      [
//        408, 8193, 0, 35, 503311, 90117, 16385, 168, 1832621, 106497, 73728, 78,
//        1483410, 8193, 0, 78, 0, 8193, 0, 78, 5, 10, 0, 64, 0, 2313, 2313, 62,
//        0, 13108, 0, 34, 2, 17477, 0, 35,
//      ],
//      [
//        408, 8193, 0, 35, 519696, 73732, 0, 168, 1906349, 32769, 0, 78, 1483410,
//        8193, 0, 78, 0, 8193, 0, 78, 5, 10, 0, 65, 2313, 13108, 0, 35, 2, 17477,
//        0, 35,
//      ],
//      [
//        408, 8193, 0, 35, 519696, 73732, 16385, 168, 1906349, 32769, 0, 78,
//        1483410, 8193, 0, 78, 0, 8193, 0, 78, 5, 10, 0, 66, 2313, 13108, 0, 35,
//        2, 6554, 0, 36, 0, 10923, 0, 35,
//      ],
//      [
//        408, 8193, 0, 35, 536081, 57347, 8192, 168, 1906349, 32769, 24577, 78,
//        1483410, 8193, 0, 78, 0, 8193, 0, 78, 5, 10, 0, 67, 2313, 13108, 0, 35,
//        2, 6554, 0, 37, 0, 10923, 0, 36,
//      ],
//      [
//        408, 8193, 0, 36, 544273, 49155, 8193, 168, 1930926, 8192, 8192, 78,
//        1483410, 8193, 0, 78, 0, 8193, 0, 78, 5, 10, 0, 68, 2313, 13108, 0, 35,
//        2, 6554, 0, 38, 0, 10923, 0, 37,
//      ],
//      [
//        408, 8193, 0, 37, 552466, 40962, 0, 168, 3422528, 8193, 0, 78, 0, 8193,
//        0, 78, 5, 10, 5, 69, 2313, 13108, 0, 35, 2, 6554, 0, 39, 0, 10923, 0,
//        38,
//      ],
//      [
//        408, 8193, 0, 38, 552466, 40962, 8192, 168, 3422528, 8193, 0, 78, 0,
//        8193, 0, 78, 10, 5, 5, 70, 2313, 13108, 0, 35, 2, 6554, 0, 40, 0, 10923,
//        0, 39,
//      ],
//      [
//        408, 8193, 0, 39, 560658, 32770, 8192, 168, 3422528, 8193, 0, 78, 0,
//        8193, 0, 78, 2328, 6554, 0, 36, 0, 6554, 0, 35, 2, 6554, 0, 41, 0,
//        10923, 0, 40,
//      ],
//      [
//        408, 8193, 0, 40, 568850, 24578, 0, 168, 3422528, 8193, 0, 78, 0, 8193,
//        0, 78, 2328, 6554, 0, 37, 0, 6554, 0, 36, 2, 6554, 0, 42, 0, 10923, 0,
//        41,
//      ],
//      [
//        408, 8193, 0, 41, 568850, 24578, 8193, 168, 3422528, 8193, 0, 78, 0,
//        8193, 0, 78, 2328, 6554, 6554, 59, 0, 6554, 0, 37, 2, 6554, 0, 46, 0,
//        10923, 0, 42,
//      ],
//      [
//        408, 8193, 0, 42, 577043, 16385, 8193, 168, 3422528, 8193, 0, 78, 0,
//        8193, 0, 78, 8882, 6554, 6554, 59, 2, 6554, 0, 39, 0, 10923, 0, 43,
//      ],
//      [
//        408, 8193, 0, 43, 585236, 8192, 0, 168, 3422528, 8193, 0, 78, 0, 8193,
//        0, 78, 15438, 6554, 0, 40, 0, 10923, 0, 44,
//      ],
//      [
//        408, 8193, 8193, 122, 585236, 8192, 8192, 168, 3422528, 8193, 0, 78, 0,
//        8193, 0, 78, 15438, 6554, 6554, 47, 0, 10923, 10923, 45,
//      ],
//      [4024557, 8193, 0, 78, 0, 8193, 0, 78],
//      [4024557, 8193, 0, 78, 0, 8193, 0, 78],
//      [4024557, 8193, 0, 78, 0, 8193, 0, 78],
//      [4024557, 8193, 8193, 78, 0, 8193, 8193, 78],
//    ],
//    numTicks: 4147083,
//    maxSelf: 507690,
//  },
//  timeline: {
//    startTime: 1642688200,
//    samples: [
//      52997, 314116, 166037, 145716, 119938, 156626, 95277, 299103, 42874,
//      103485, 78324, 50019, 93481, 257538, 246101, 75713, 127838, 89530, 112918,
//      268927, 79218, 61979, 117654, 87601, 88796, 316884, 103538, 131815,
//      113310, 149760, 0,
//    ],
//    durationDelta: 10,
//    watermarks: {},
//  },
//  metadata: {
//    format: 'single',
//    spyName: 'gospy',
//    sampleRate: 100,
//    units: 'objects',
//    appName: 'pyroscope.server.alloc_objects',
//    startTime: 1642688208,
//    endTime: 1642688508,
//    query: 'pyroscope.server.alloc_objects{}',
//    maxNodes: 1024,
//  },
// };

const TestData: Profile = {
  version: 1,
  flamebearer: {
    names: [
      'total',
      'runtime.main',
      'main.main',
      'github.com/pyroscope-io/client/pyroscope.TagWrapper',
      'runtime/pprof.Do',
      'github.com/pyroscope-io/client/pyroscope.TagWrapper.func1',
      'main.main.func1',
      'main.slowFunction',
      'main.slowFunction.func1',
      'main.work',
      'main.fastFunction',
      'main.fastFunction.func1',
    ],
    levels: [
      [0, 95, 0, 0],
      [0, 95, 0, 1],
      [0, 95, 0, 2],
      [0, 95, 0, 3],
      [0, 95, 0, 4],
      [0, 95, 0, 5],
      [0, 95, 0, 6],
      [0, 19, 0, 10, 0, 76, 0, 7],
      [0, 19, 0, 3, 0, 76, 0, 4],
      [0, 19, 0, 4, 0, 76, 0, 8],
      [0, 19, 0, 5, 0, 76, 76, 9],
      [0, 19, 0, 11],
      [0, 19, 19, 9],
    ],
    numTicks: 95,
    maxSelf: 76,
  },
  metadata: {
    format: 'single',
    spyName: 'gospy',
    sampleRate: 100,
    units: 'samples',
    name: 'simple.golang.app.cpu 2022-03-09T20:25:55Z',
    appName: 'simple.golang.app.cpu',
    startTime: 1646857555,
    endTime: 1646857555,
    query: 'simple.golang.app.cpu{}',
    maxNodes: 8192,
  },
};
