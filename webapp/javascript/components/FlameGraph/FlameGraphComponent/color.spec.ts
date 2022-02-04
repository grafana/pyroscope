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

  describe.only('dotnetspy', () => {
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
});
