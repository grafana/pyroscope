{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='limitedPriorityLevelConfiguration', url='', help='"LimitedPriorityLevelConfiguration specifies how to handle requests that are subject to limits. It addresses two issues:\\n * How are requests for this priority level limited?\\n * What should be done with requests that exceed the limit?"'),
  '#limitResponse':: d.obj(help='"LimitResponse defines how to handle requests that can not be executed right now."'),
  limitResponse: {
    '#queuing':: d.obj(help='"QueuingConfiguration holds the configuration parameters for queuing"'),
    queuing: {
      '#withHandSize':: d.fn(help="\"`handSize` is a small positive number that configures the shuffle sharding of requests into queues.  When enqueuing a request at this priority level the request's flow identifier (a string pair) is hashed and the hash value is used to shuffle the list of queues and deal a hand of the size specified here.  The request is put into one of the shortest queues in that hand. `handSize` must be no larger than `queues`, and should be significantly smaller (so that a few heavy flows do not saturate most of the queues).  See the user-facing documentation for more extensive guidance on setting this field.  This field has a default value of 8.\"", args=[d.arg(name='handSize', type=d.T.integer)]),
      withHandSize(handSize): { limitResponse+: { queuing+: { handSize: handSize } } },
      '#withQueueLengthLimit':: d.fn(help='"`queueLengthLimit` is the maximum number of requests allowed to be waiting in a given queue of this priority level at a time; excess requests are rejected.  This value must be positive.  If not specified, it will be defaulted to 50."', args=[d.arg(name='queueLengthLimit', type=d.T.integer)]),
      withQueueLengthLimit(queueLengthLimit): { limitResponse+: { queuing+: { queueLengthLimit: queueLengthLimit } } },
      '#withQueues':: d.fn(help='"`queues` is the number of queues for this priority level. The queues exist independently at each apiserver. The value must be positive.  Setting it to 1 effectively precludes shufflesharding and thus makes the distinguisher method of associated flow schemas irrelevant.  This field has a default value of 64."', args=[d.arg(name='queues', type=d.T.integer)]),
      withQueues(queues): { limitResponse+: { queuing+: { queues: queues } } },
    },
    '#withType':: d.fn(help='"`type` is \\"Queue\\" or \\"Reject\\". \\"Queue\\" means that requests that can not be executed upon arrival are held in a queue until they can be executed or a queuing limit is reached. \\"Reject\\" means that requests that can not be executed upon arrival are rejected. Required."', args=[d.arg(name='type', type=d.T.string)]),
    withType(type): { limitResponse+: { type: type } },
  },
  '#withAssuredConcurrencyShares':: d.fn(help="\"`assuredConcurrencyShares` (ACS) configures the execution limit, which is a limit on the number of requests of this priority level that may be exeucting at a given time.  ACS must be a positive number. The server's concurrency limit (SCL) is divided among the concurrency-controlled priority levels in proportion to their assured concurrency shares. This produces the assured concurrency value (ACV) --- the number of requests that may be executing at a time --- for each such priority level:\\n\\n            ACV(l) = ceil( SCL * ACS(l) / ( sum[priority levels k] ACS(k) ) )\\n\\nbigger numbers of ACS mean more reserved concurrent requests (at the expense of every other PL). This field has a default value of 30.\"", args=[d.arg(name='assuredConcurrencyShares', type=d.T.integer)]),
  withAssuredConcurrencyShares(assuredConcurrencyShares): { assuredConcurrencyShares: assuredConcurrencyShares },
  '#mixin': 'ignore',
  mixin: self,
}
