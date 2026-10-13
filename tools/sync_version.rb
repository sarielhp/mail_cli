#!/usr/bin/env ruby
# frozen_string_literal: true

# VERSION is the release version's source of truth. app/constants.go embeds it
# as the compiled-in Version constant and docs/clihelp prints it, so tools/bump
# runs this (with no argument) to write VERSION into the constant, and the fast
# gate runs it with --check, so the file and the constant cannot drift apart.
root = File.expand_path('..', __dir__)
Dir.chdir(root)

CONSTANT = 'app/constants.go'
PATTERN = /(\bVersion\s*=\s*)"([^"]*)"/

def usage
  abort 'Usage: tools/sync_version.rb [--check]'
end

def version_value
  version = File.read('VERSION').strip
  abort "VERSION is not a semantic version: #{version.inspect}" unless version.match?(/\A\d+\.\d+\.\d+\z/)

  version
end

def constant_version
  source = File.read(CONSTANT)
  match = PATTERN.match(source)
  abort "#{CONSTANT}: no Version constant found" unless match

  [source, match[2]]
end

usage unless ARGV.empty? || ARGV == ['--check']
version = version_value
source, current = constant_version

if ARGV == ['--check']
  abort "#{CONSTANT}: Version = #{current} but VERSION = #{version}; run ruby tools/sync_version.rb" if current != version

  puts "VERSION and #{CONSTANT} agree on #{version}"
else
  File.write(CONSTANT, source.sub(PATTERN) { "#{Regexp.last_match(1)}\"#{version}\"" })
  puts "#{CONSTANT}: Version = #{version}"
end
