{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='limitResponse', url='', help='"LimitResponse defines how to handle requests that can not be executed right now."'),
  '#queuing':: d.obj(help='"QueuingConfiguration holds the configuration parameters for queuing"'),
  queuing: {
    '#withHandSize':: d.fn(help="\"`handSize` is a small positive number that configures the shuffle sharding of requests into queues.  When enqueuing a request at this priority level the request's flow identifier (a string pair) is hashed and the hash value is used to shuffle the list of queues and deal a hand of the size specified here.  The request is put into one of the shortest queues in that hand. `handSize` must be no larger than `queues`, and should be significantly smaller (so that a few heavy flows do not saturate most of the queues).  See the user-facing documentation for more extensive guidance on setting this field.  This field has a default value of 8.\"", args=[d.arg(name='handSize', type=d.T.integer)]),
    withHandSize(handSize): { queuing+: { handSize: handSize } },
    '#withQueueLengthLimit':: d.fn(help='"`queueLengthLimit` is the maximum number of requests allowed to be waiting in a given queue of this priority level at a time; excess requests are rejected.  This value must be positive.  If not specified, it will be defaulted to 50."', args=[d.arg(name='queueLengthLimit', type=d.T.integer)]),
    withQueueLengthLimit(queueLengthLimit): { queuing+: { queueLengthLimit: queueLengthLimit } },
    '#withQueues':: d.fn(help='"`queues` is the number of queues for this priority level. The queues exist independently at each apiserver. The value must be positive.  Setting it to 1 effectively precludes shufflesharding and thus makes the distinguisher method of associated flow schemas irrelevant.  This field has a default value of 64."', args=[d.arg(name='queues', type=d.T.integer)]),
    withQueues(queues): { queuing+: { queues: queues } },
  },
  '#withType':: d.fn(help='"`type` is \\"Queue\\" or \\"Reject\\". \\"Queue\\" means that requests that can not be executed upon arrival are held in a queue until they can be executed or a queuing limit is reached. \\"Reject\\" means that requests that can not be executed upon arrival are rejected. Required."', args=[d.arg(name='type', type=d.T.string)]),
  withType(type): { type: type },
  '#mixin': 'ignore',
  mixin: self,
}
