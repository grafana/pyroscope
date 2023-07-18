{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='statusCause', url='', help='"StatusCause provides more information about an api.Status failure, including cases when multiple errors are encountered."'),
  '#withField':: d.fn(help='"The field of the resource that has caused this error, as named by its JSON serialization. May include dot and postfix notation for nested attributes. Arrays are zero-indexed.  Fields may appear more than once in an array of causes due to fields having multiple errors. Optional.\\n\\nExamples:\\n  \\"name\\" - the field \\"name\\" on the current resource\\n  \\"items[0].name\\" - the field \\"name\\" on the first array entry in \\"items\\', args=[d.arg(name='field', type=d.T.string)]),
  withField(field): { field: field },
  '#withMessage':: d.fn(help='"A human-readable description of the cause of the error.  This field may be presented as-is to a reader."', args=[d.arg(name='message', type=d.T.string)]),
  withMessage(message): { message: message },
  '#withReason':: d.fn(help='"A machine-readable description of the cause of the error. If this value is empty there is no information available."', args=[d.arg(name='reason', type=d.T.string)]),
  withReason(reason): { reason: reason },
  '#mixin': 'ignore',
  mixin: self,
}
