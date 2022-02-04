import Color from 'color';
import {
  colorBasedOnDiffPercent,
  NewDiffColor,
  getPackageNameFromStackTrace,
} from './color';
import { DefaultPalette } from './colorPalette';

describe.each([
  // red (diff > 0)
  [30, 60, DefaultPalette.badColor.toString()],

  // green (diff < 0%)
  [60, 0, DefaultPalette.goodColor.toString()],

  // grey (diff == 0)
  [0, 0, DefaultPalette.neutralColor.toString()],
])('.colorBasedOnDiffPercent(%i, %i)', (a, b, expected) => {
  it(`returns ${expected}`, () => {
    expect(colorBasedOnDiffPercent(DefaultPalette, a, b).rgb().toString()).toBe(
      expected
    );
  });
});

describe('NewDiffColor with white-to-black example palette', () => {
  describe.each([
    [-100, 'rgb(255, 255, 255)'],
    [0, 'rgb(128, 128, 128)'],
    [100, 'rgb(0, 0, 0)'],
  ])('.NewDiffColor(%i)', (a, expected) => {
    it(`returns ${expected}`, () => {
      const color = NewDiffColor({
        name: 'my palette',
        goodColor: Color('white'),
        neutralColor: Color('grey'),
        badColor: Color('black'),
      });

      expect(color(a).rgb().toString()).toBe(expected);
    });
  });
});

describe.only('getPackageNameFromStackTrace', () => {
  describe('golang', () => {
    describe.each([
      ['bufio.(*Reader).fill', 'bufio.'],
      ['cmpbody', 'cmpbody'],
      ['bytes.Compare', 'bytes.'],
      ['crypto/tls.(*Conn).clientHandshake', 'crypto/tls.'],
      [
        'github.com/DataDog/zstd._Cfunc_ZSTD_compress_wrapper',
        'github.com/DataDog/zstd.',
      ],
      [
        'github.com/dgraph-io/badger/v2.(*DB).calculateSize',
        'github.com/dgraph-io/badger/v2.',
      ],
      [
        'github.com/dgraph-io/badger/v2/table.(*blockIterator).next',
        'github.com/dgraph-io/badger/v2/table.',
      ],
      ['path/filepath.walk', 'path/filepath.'],
      ['os.(*File).write', 'os.'],
    ])(`.getPackageNameFromStackTrace('%s')`, (a, expected) => {
      it(`returns '${expected}'`, () => {
        expect(getPackageNameFromStackTrace('gospy', a)).toBe(expected);
      });
    });
  });

  describe('pyspy', () => {
    describe.each([
      ['total', 'total'],
      [
        'System.Private.CoreLib!System.Threading.TimerQueue.FireNextTimers()',
        'System.Private.CoreLib!System.Threading',
      ],
      [
        'StackExchange.Redis!StackExchange.Redis.ConnectionMultiplexer.OnHeartbeat()',
        'StackExchange.Redis!StackExchange.Redis',
      ],
      [
        'Microsoft.AspNetCore.Server.Kestrel.Core!Microsoft.AspNetCore.Server.Kestrel.Core.Internal.Http.HttpRequestPipeReader.ReadAsync(value class System.Threading.CancellationToken)',
        'Microsoft.AspNetCore.Server.Kestrel.Core!Microsoft.AspNetCore.Server.Kestrel.Core.Internal.Http',
      ],
      [
        'Google.Protobuf!Google.Protobuf.ParsingPrimitivesMessages.ReadRawMessage(value class Google.Protobuf.ParseContext\u0026,class Google.Protobuf.IMessage)',
        'Google.Protobuf!Google.Protobuf',
      ],
      [
        'Grpc.AspNetCore.Server!Grpc.AspNetCore.Server.Internal.PipeExtensions.ReadSingleMessageAsync(class System.IO.Pipelines.PipeReader,class Grpc.AspNetCore.Server.Internal.HttpContextServerCallContext,class System.Func`2\u003cclass Grpc.Core.DeserializationContext,!!0\u003e)',
        'Grpc.AspNetCore.Server!Grpc.AspNetCore.Server.Internal',
      ],
      [
        'System.Private.CoreLib!System.Runtime.CompilerServices.AsyncTaskMethodBuilder`1[System.__Canon].GetStateMachineBox(!!0\u0026,class System.Threading.Tasks.Task`1\u003c!0\u003e\u0026)',
        'System.Private.CoreLib!System.Runtime.CompilerServices.AsyncTaskMethodBuilder`1[System',
      ],
    ])(`.getPackageNameFromStackTrace('%s')`, (a, expected) => {
      it(`returns '${expected}'`, () => {
        expect(getPackageNameFromStackTrace('dotnetspy', a)).toBe(expected);
      });
    });
  });

  describe('pyspy', () => {
    describe.each([
      ['total', 'total'],
      ['urllib3/response.py:579 - stream', 'urllib3/'],
      ['requests/models.py:580 - prepare_cookies', 'requests/'],
      ['logging/__init__.py:1548 - findCaller', 'logging/'],
      [
        'jaeger_client/thrift_gen/jaeger/ttypes.py:147 - write',
        'jaeger_client/thrift_gen/jaeger/',
      ],

      // TODO: this one looks incorrect, but keeping in the test for now
      [
        '\u003cfrozen importlib._bootstrap\u003e:1030 - _gcd_import',
        '<frozen importlib._bootstrap>:1030 - _gcd_import',
      ],
    ])(`.getPackageNameFromStackTrace('%s')`, (a, expected) => {
      it(`returns '${expected}'`, () => {
        expect(getPackageNameFromStackTrace('pyspy', a)).toBe(expected);
      });
    });
  });

  describe('rbspy', () => {
    describe.each([
      ['total', 'total'],
      ['webrick/utils.rb:194 - watch', 'webrick/'],
      ['webrick/server.rb:190 - block (2 levels) in start', 'webrick/'],
      [
        'gems/sinatra-2.0.3/lib/sinatra/base.rb:1537 - start_server',
        'gems/sinatra-2.0.3/lib/sinatra/',
      ],
      ['services/driver/client.rb:34 - get_drivers', 'services/driver/'],
      ['uri/common.rb:742 - URI', 'uri/'],
      ['net/protocol.rb:299 - block in write0', 'net/'],
    ])(`.getPackageNameFromStackTrace('%s')`, (a, expected) => {
      it(`returns '${expected}'`, () => {
        expect(getPackageNameFromStackTrace('rbspy', a)).toBe(expected);
      });
    });
  });

  describe.only('phpspy', () => {
    describe.each([
      ['total', 'total'],
      ['<internal> - sleep', '<internal> - sleep'],
      // Those were copied from phpspy documentation
      // and were not tested with pyroscope
      // So I can't attest to their correctness
      // https://github.com/adsr/phpspy#example-pgrep-daemon-mode
      [
        'Cache_MemcachedToggleable::get /foo/bar/lib/Cache/MemcachedToggleable.php:26',
        'Cache_MemcachedToggleable::get /foo/bar/lib/Cache/',
      ],
      [
        '/foo/bar/lib/Security/Rule/Engine.php:210',
        '/foo/bar/lib/Security/Rule/',
      ],
    ])(`.getPackageNameFromStackTrace('%s')`, (a, expected) => {
      it(`returns '${expected}'`, () => {
        expect(getPackageNameFromStackTrace('phpspy', a)).toBe(expected);
      });
    });
  });
});
