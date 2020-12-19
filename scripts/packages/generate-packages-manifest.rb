#!/usr/bin/env ruby
require 'aws-sdk-s3'
require 'json'
require 'octokit'
require 'yaml'

auth_info = YAML.load(File.read(File.join(ENV["HOME"], "/.config/gh/hosts.yml")))

github = Octokit::Client.new(:access_token => auth_info["github.com"]["oauth_token"])
releases = github.releases('pyroscope-io/pyroscope').map do |release|
  release = release.to_h
  [release[:tag_name], release[:created_at]]
end.to_h

s3 = Aws::S3::Client.new
sha_mapping = s3.list_objects(:bucket => 'dl.pyroscope.io').flat_map(&:contents).map do |object|
  sha = s3.head_object(:bucket => 'dl.pyroscope.io', :key => object.key).metadata["sha256"]
  next unless sha

  [File.basename(object.key), sha]
end.compact.to_h

json = JSON.pretty_generate({
  "releases": releases,
  "shaMapping": sha_mapping
})
puts json
File.write("scripts/packages/packages.manifest.json", json)


