#!/usr/bin/env ruby
# frozen_string_literal: true

# Architecture guard: the layering the modularity rewrite established, checked
# as cheaply as it can be. Each rule records what it prevents, because a bare
# failure message is not enough to act on a year from now.
#
# Run on its own, or as a fast check of tools/gate.

require 'open3'

root = File.expand_path('..', __dir__)
Dir.chdir(root)

FAILURES = []

# The module path, so the rules below name packages the way `go list` prints
# them, whatever the module is called.
MODULE = Open3.capture3('go', 'list', '-m').first.strip

def fail_rule(rule, why, offenders)
  FAILURES << [rule, why, offenders]
end

# Packages below the user interface: they report, they do not print.
BELOW_UI = %w[backend mailclient cache email calendar contacts label report spam scan
              organize show last sieve train message_info terminal browser outbox watcher pgp].freeze

# 1. No presentation below the UI layer.
offenders = []
BELOW_UI.each do |dir|
  next unless Dir.exist?(dir)

  out, = Open3.capture3("grep -rn 'fmt\\.Print\\|Color[A-Za-z]*\\.Print' --include='*.go' #{dir}")
  out.lines.each do |line|
    next if line =~ /_test\.go:/
    # Prose about the rule is not a violation of it.
    next if line =~ /:\s*(\/\/|\*)/

    offenders << line.strip
  end
end
unless offenders.empty?
  fail_rule('no printing below the UI',
            'These layers must report through report.Reporter, or write to an io.Writer ' \
            'the caller supplies. Printing here cannot be redirected, cannot be asserted ' \
            'on without swapping os.Stdout, and corrupts the TUI display.',
            offenders)
end

# 2. No UI imports below the UI layer.
out, = Open3.capture3("go list -f '{{.ImportPath}} {{join .Imports \" \"}}' ./backend/... ./mailclient/... ./cache/...")
offenders = out.lines.select { |l| l =~ %r{#{Regexp.escape(MODULE)}/(ui|tui)(\s|$)} }.map(&:strip)
unless offenders.empty?
  fail_rule('no UI imports below the UI',
            'A backend that imports a rendering package cannot be used by any other front end.',
            offenders)
end

# 3. Only mailops may name a concrete backend.
out, = Open3.capture3(
  "go list -f '{{.ImportPath}} {{join .Imports \" \"}}' ./... 2>/dev/null"
)
offenders = out.lines.select do |l|
  path = l.split.first.to_s
  l.include?("#{MODULE}/backend/") &&
    !path.start_with?("#{MODULE}/backend") &&
    path != "#{MODULE}/mailops"
end.map { |l| l.split.first }
unless offenders.empty?
  fail_rule('only mailops names a backend',
            'Choosing a backend is the factory\'s job. Importing gmail, outlook, jmap or ' \
            'mbox anywhere else reintroduces the coupling AccountOps and mailops exist to remove.',
            offenders)
end

# 4. No CLI flag state below the CLI.
out, = Open3.capture3("grep -rn 'app\\.Flag' --include='*.go' .")
offenders = out.lines.reject do |l|
  l =~ %r{^\./cli/} || l =~ %r{^\./app/} || l =~ /_test\.go:/ ||
    l =~ %r{:\s*(//|\*)} || l =~ %r{^\./tools/}
end.map(&:strip)
unless offenders.empty?
  fail_rule('no CLI flags below the CLI',
            'Reading the parser\'s globals makes a package impossible to call with ' \
            'different options from the TUI, or twice in one test. Pass an options struct.',
            offenders)
end

if FAILURES.empty?
  puts "\e[32m✔ Architecture check passed\e[0m"
  exit 0
end

FAILURES.each do |rule, why, offenders|
  puts "\e[31m✗ #{rule}\e[0m"
  puts "  #{why}"
  offenders.first(20).each { |o| puts "    #{o}" }
  puts "    ... and #{offenders.size - 20} more" if offenders.size > 20
  puts
end
exit 1
