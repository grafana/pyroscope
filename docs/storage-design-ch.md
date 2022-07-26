![image](https://user-images.githubusercontent.com/23323466/110414341-8ad0c000-8044-11eb-9628-7b24e50295b2.png)

# O(log n)让持续分析成为可能

Pyroscope是一种软件，可让你**持续地**分析代码以调试每行代码的性能问题。仅需几行代码，它即可执行以下操作：

### Pyroscope客户端
- 以0.01秒为单位检测堆栈跟踪，以查看哪些函数正在消耗资源
- 将数据批量处理为10秒的数据块并发送给Pyroscope服务器

### Pyroscope服务器
- 接收Pyroscope客户端发来的数据并对其进行处理以便高效存储
- 对分析数据进行预汇总，以便在需要检索数据时能快速查询

## 存储效率

持续分析的挑战在于，如果你只是频繁地将分析数据块压缩并存储在某个地方，它将导致:
1.	太多数据以致无法有效存储
2.	太多数据以致无法快速查询

我们可用以下方式解决这些问题：
1.	用字典树和树的组合来高效压缩数据
2.	用线段树在O(log n)时间内返回任何时间段内的数据查询结果，而不是 O(n)的时间复杂度

## 步骤 1: 将分析数据转换为树

呈现分析数据的最简单方式是：在一个字符串列表中，每个字符串表示一个堆栈跟踪和该特定堆栈追踪在某个分析段中被看到的次数：

```bash
server.py;fast_function;work 2
server.py;slow_function;work 8
```

我们要做的第一件事就是把这些数据转换为树。方便的是，该呈现方式也使后续生成火焰图变得更加容易。

![raw_vs_flame_graph](https://user-images.githubusercontent.com/23323466/110378930-0f065180-800b-11eb-9357-71724bc7258c.gif)

将堆栈跟踪压缩为树可以在重复元素上节省空间。通过使用树，我们从原来的必须在数据库中多次存储诸如`net/http.request`之类的通用路径，到现在只需要存储该通用路径一次并保存它的存储位置的位置引用就可以了。这种存储方式在性能分析库中基本是标配，因为它是分析数据存储的优化中可最轻松实施的方法。

![fast-compress-stack-traces](https://user-images.githubusercontent.com/23323466/110227218-e109fb80-7eaa-11eb-81a8-cdf2b3944f1c.gif)

## 步骤 2: 添加字典树以更有效地存储单个符号

目前，我们已通过转换为树来压缩原始分析数据，但该压缩树中有很多节点包含与其他节点共享重复元素的符号，例如：

```
net/http.request;net/io.read 100 samples
net/http.request;net/io.write 200 samples
```

尽管 `net/http.request`, `net/io.read`, 和`net/io.write`函数不同，但是`net/`是他们共同的祖先。

如下所示，每一行都可以使用前缀树的方式来序列化。这意味着不再需要多次存储相同的前缀，现在我们只需在字典树中存储它们一次，并通过储存内存地址的指针变量来访问它们:

![storage-design-0](https://user-images.githubusercontent.com/23323466/110520399-446e7600-80c3-11eb-84e9-ecac7c0dbf23.gif)

在这个基本示例中，我们从39字节减少到8字节，节省了约80%的空间。通常，符号名称要长得多，而且随着符号数量的增加，存储需求呈对数增长而非线性增长。

## 步骤 1 + 2: 将树和字典树结合

最后，通过使用树来压缩原始分析数据，然后使用字典树来压缩符号，我们基础示例的存储空间如下：

```
| 数据类型             |  字节  |
|---------------------|-------|
| 原始数据             | 93    |
| 树                  | 58    |
| 树 + 字典树          | 10    |
```

如你所见，对于该基础示例，这是一个9倍的改进。实际情景中，压缩因子会变得更大。

![combine-segment-and-prefix_1](https://user-images.githubusercontent.com/23323466/110262208-ca75aa00-7f67-11eb-8f16-0572a4641ee1.gif)

## 步骤 3：使用线段树优化以便快速读取

现在，我们已经有了有效存储数据的方法，接下来的问题是如何高效地查询数据。我们解决该问题的方法是把分析数据预先汇总，并将其存储在一个特殊的线段树中。

每10秒，Pyroscope客户端就会发送一个分析数据块到服务器，该服务器将数据和其相应的时间戳写入数据库中。你将注意到，每次写入只发生一次，但会被复制多次。

**每一层代表一个更大单位的时间块，所以在这种情况下，每两个10秒的时间块将创建一个20秒的时间块。这是为了使读取数据更加高效 (稍后详细介绍)**。

![segment_tree_animation_1](https://user-images.githubusercontent.com/23323466/110259555-196a1200-7f5d-11eb-9223-218bb4b34c6b.gif)

## 将读取从O（n）优化到O（log n）

如果你不使用线段树，只是将10秒为单位的数据块写入数据库，读取数据的时间复杂度将是基于以10秒为单位的查询请求的一个函数。如果，你想要提取1年的数据，那么你必须合并用来呈现分析数据的3,154,000棵树。通过使用线段树，你可以有效地将合并操作的数量从O（n）减少到O（log n）。

![segment_tree_reads](https://user-images.githubusercontent.com/23323466/110277713-b98a6000-7f8a-11eb-942f-3a924a6e0b09.gif)


## 帮助我们添加更多分析工具

我们花了很多时间来解决存储/查询问题，因为我们希望可以让软件在生产环境中也能真正地进行持续性能分析,而且不会造成太多虚耗。

虽然，目前Pyroscope支持4种语言，但我们希望能添加更多语言。

任何能够以上面链接中的“原始”格式导出数据的采样分析器都有可能成为Pyroscope的分析客户端。我们热切地希望你能帮助我们开发其他语言的分析工具!


- [x] [Go](https://pyroscope.io/docs/golang)
- [x] [Python](https://pyroscope.io/docs/python)
- [x] [eBPF](https://pyroscope.io/docs/ebpf)
- [x] [Ruby](https://pyroscope.io/docs/ruby)
- [x] [PHP](https://pyroscope.io/docs/php)
- [x] [Java](https://pyroscope.io/docs/java)
- [x] [.NET](https://pyroscope.io/docs/dotnet)
- [ ] [Rust](https://github.com/pyroscope-io/pyroscope/issues/83#issuecomment-784947654)
- [ ] [Node](https://github.com/pyroscope-io/pyroscope/issues/8)

如果你想做出贡献或需要Pyroscope设置方面的帮助，你可以通过以下方式联系我们:
- 加入我们的[Slack](https://pyroscope.io/slack)
- 通过[此链接](https://pyroscope.io/setup-call)与我们预约会面
- 提交[问题报告](https://github.com/pyroscope-io/pyroscope/issues)
- 在[Twitter](https://twitter.com/PyroscopeIO)上关注我们
