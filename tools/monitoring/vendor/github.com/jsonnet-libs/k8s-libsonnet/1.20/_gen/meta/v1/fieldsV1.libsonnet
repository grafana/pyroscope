{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='fieldsV1', url='', help="\"FieldsV1 stores a set of fields in a data structure like a Trie, in JSON format.\\n\\nEach key is either a '.' representing the field itself, and will always map to an empty set, or a string representing a sub-field or item. The string will follow one of these four formats: 'f:\u003cname\u003e', where \u003cname\u003e is the name of a field in a struct, or key in a map 'v:\u003cvalue\u003e', where \u003cvalue\u003e is the exact json formatted value of a list item 'i:\u003cindex\u003e', where \u003cindex\u003e is position of a item in a list 'k:\u003ckeys\u003e', where \u003ckeys\u003e is a map of  a list item's key fields to their unique values If a key maps to an empty Fields value, the field that key represents is part of the set.\\n\\nThe exact format is defined in sigs.k8s.io/structured-merge-diff\""),
  '#mixin': 'ignore',
  mixin: self,
}
