#!/usr/bin/env ruby

require 'json'

# system "curl \
# --silent \
# -H \"Accept: application/vnd.github+json\" \
# -H \"Authorization: Bearer #{ENV["GITHUB_TOKEN"]}\" \
# https://api.github.com/repos/pyroscope-io/cloudstorage/actions/workflows/34992245/runs"

latest_run = JSON.parse(`curl \
  --silent \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer #{ENV["GITHUB_TOKEN"]}" \
  https://api.github.com/repos/pyroscope-io/cloudstorage/actions/workflows/34992245/runs`)["workflow_runs"][0]

puts "latest run: #{latest_run ? latest_run["id"] : "none"}"


puts "triggering a run"
system "curl \
-X POST \
-H \"Accept: application/vnd.github+json\" \
-H \"Authorization: Bearer #{ENV["GITHUB_TOKEN"]}\" \
https://api.github.com/repos/pyroscope-io/cloudstorage/actions/workflows/34992245/dispatches \
-d '{\"ref\":\"main\",\"inputs\":{\"gitRef\":\"#{ENV["HEAD_REF"] || "main"}\"}'"

# TODO: this logic is prone to race conditions, consider improving
run_id = nil
10.times do
  runs = JSON.parse(`curl \
  --silent \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer #{ENV["GITHUB_TOKEN"]}" \
  https://api.github.com/repos/pyroscope-io/cloudstorage/actions/workflows/34992245/runs`)["workflow_runs"]

  run = runs.find { |r| latest_run.nil? || r["created_at"] > latest_run["created_at"] }
  if run
    run_id = run["id"]
    break
  end
end

raise "could not find run" unless run_id

puts "current run_id: #{run_id}"

60.times do
  status=JSON.parse(`curl \
  --silent \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer #{ENV["GITHUB_TOKEN"]}" \
  https://api.github.com/repos/pyroscope-io/cloudstorage/actions/runs/#{run_id}`)["status"]

  puts "status: #{status}"
  break if status == "completed"
  sleep 10
end

run = JSON.parse(`curl \
--silent \
-H "Accept: application/vnd.github+json" \
-H "Authorization: Bearer #{ENV["GITHUB_TOKEN"]}" \
https://api.github.com/repos/pyroscope-io/cloudstorage/actions/runs/#{run_id}`)

conclusion=run["conclusion"]

puts "conclusion: #{conclusion}"

puts ""
puts "---"
puts ""

if conclusion != "success"
  puts "This version of pyroscope OSS is not compatible with downstream Pyroscope Cloud project"
  puts "Go to https://github.com/pyroscope-io/cloudstorage/actions/runs/#{run_id} for more information"
  # exit 1
end
