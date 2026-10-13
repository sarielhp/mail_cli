# frozen_string_literal: true

require 'digest'
require 'fileutils'
require 'json'
require 'open3'
require_relative 'settings'

# Copied beside the entry points; no runtime dependency on the standards checkout.
module Tooling
  ANSI = /\e\[[\d;]*m/
  VERSION = /\A(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)\z/
  DEEP_TOOLS = %w[govulncheck gocritic shadow revive dupl].freeze

  module_function

  def setup
    Dir.chdir(File.realpath(File.expand_path('..', __dir__)))
    abort 'go.mod missing; adapt these tools in a Go repository' unless File.file?('go.mod')
    ENV['PATH'] = search_paths.join(File::PATH_SEPARATOR)
  end

  def search_paths
    configured = [ENV.fetch('GOBIN', nil), ENV['GOPATH']&.split(File::PATH_SEPARATOR)&.map { |path| File.join(path, 'bin') }]
    (configured.flatten.compact + ENV.fetch('PATH', '').split(File::PATH_SEPARATOR) +
      [File.expand_path('~/.go/bin'), File.expand_path('~/go/bin')]).uniq
  end

  def executable(name)
    if ToolSettings::TOOLS.key?(name)
      path = ToolSettings::TOOLS.fetch(name)
      raise "required tool missing: #{name} at #{path}" unless File.file?(path) && File.executable?(path)

      return File.expand_path(path)
    end
    path = search_paths.map { |dir| File.join(dir, name) }.find { |file| File.file?(file) && File.executable?(file) }
    raise "required tool missing: #{name}; bootstrap separately with authorization" unless path

    File.expand_path(path)
  end

  def capture(*args)
    Open3.capture3([args.first, args.first], *args.drop(1))
  rescue SystemCallError => e
    raise "cannot execute #{args.first}: #{e.message}"
  end

  def run(*args)
    out, err, status = capture(*args)
    $stdout.write(out)
    $stderr.write(err)
    raise "command failed (#{status.exitstatus || status.termsig}): #{args.inspect}" unless status.success?

    out
  end

  def git(*args)
    out, err, status = capture('git', *args)
    raise "git #{args.first} failed: #{out}#{err}" unless status.success?

    out
  end

  def script(name, *)
    path = File.join(Dir.pwd, 'tools', name)
    raise "required entry point missing or not executable: tools/#{name}" unless File.file?(path) && File.executable?(path)

    run(path, *)
  end

  def contained_path(path)
    root = Dir.pwd
    target = File.expand_path(path)
    raise "artifact must be inside repository: #{path}" unless target.start_with?("#{root}/")

    ancestor = target
    ancestor = File.dirname(ancestor) until File.exist?(ancestor) || File.symlink?(ancestor)
    resolved = File.realpath(ancestor)
    raise "artifact symlink escapes repository: #{path}" unless resolved == root || resolved.start_with?("#{root}/")

    forbidden = target.include?('/.git/') || target == File.join(root, '.git') || target.start_with?("#{root}/tools/")
    forbidden ||= %w[.go .mod .sum].include?(File.extname(target))
    raise "artifact cannot target source/control files: #{path}" if forbidden
    raise "artifact target cannot be a symlink: #{path}" if File.symlink?(target)

    target
  end

  def artifacts
    return ToolSettings::ARTIFACTS unless ToolSettings::ARTIFACTS.nil?
    return {} if Dir.glob('*.go').all? { |path| path.end_with?('_test.go') }

    name = run(executable('go'), 'list', '-f', '{{.Name}}', '.').strip
    return {} unless name == 'main'

    { File.join('bin', File.basename(Dir.pwd)) => '.' }
  end

  def build
    outputs = artifacts.transform_keys { |path| contained_path(path) }
    run(executable('go'), 'build', *ToolSettings::BUILD_PACKAGES)
    outputs.each do |path, package|
      FileUtils.mkdir_p(File.dirname(path))
      run(executable('go'), 'build', '-o', path, package)
    end
    puts 'Build passed'
  end

  def findings(lines)
    lines.map(&:strip).reject(&:empty?).tally
  end

  def baseline_check(name, current)
    baseline = "tools/#{name}-baseline.txt"
    allowed = File.file?(baseline) ? findings(File.readlines(baseline, chomp: true)) : {}
    fresh = current.flat_map { |line, count| [line] * [count - allowed.fetch(line, 0), 0].max }
    raise "#{name}: new findings:\n#{fresh.join("\n")}" unless fresh.empty?

    puts "#{name}: #{current.values.sum} findings accepted by explicit baseline" unless current.empty?
  end

  def staticcheck
    out, err, status = capture(executable('staticcheck'), '-f', 'json', './...')
    raise "staticcheck execution failed: #{out}#{err}" unless [0, 1].include?(status.exitstatus) && err.empty?

    records = out.lines.map { |line| JSON.parse(line) }
    raise 'staticcheck failed without findings' if !status.success? && records.empty?
    raise "staticcheck package loading/compile failure:\n#{out}" if records.any? { |issue| issue.fetch('code') == 'compile' }

    issues = records.map { |issue| JSON.generate(issue) }
    baseline_check('staticcheck', findings(issues))
  end

  def audit
    out, err, status = capture(executable('go-audit'), '--strict', '--quiet', '.')
    clean = out.gsub(ANSI, '')
    issues = clean.lines.select { |line| line.include?('[FAIL]') }.map(&:strip)
    expected = err.gsub(ANSI, '').strip.match?(/\AError: \d+ file\(s\) and \d+ function\(s\) exceed their hard limit in strict mode\.\z/)
    valid = status.success? && err.empty?
    valid ||= status.exitstatus == 1 && expected && !issues.empty?
    raise "go-audit execution failed: #{out}#{err}" unless valid

    baseline_check('go-audit', findings(issues))
  end

  def lint
    %w[go staticcheck go-audit].each { |name| executable(name) }
    run(executable('go'), 'vet', './...')
    staticcheck
    audit
    puts 'Lint passed'
  end

  def deep_review
    run(executable('govulncheck'), './...')
    out = run(executable('gocritic'), 'check', './...')
    raise "gocritic findings:\n#{out}" unless out.strip.empty?

    run(executable('go'), 'vet', "-vettool=#{executable('shadow')}", './...')
    run(executable('revive'), '-set_exit_status', '-exclude', 'vendor/...', './...')
    out = run(executable('dupl'), '-threshold', '50', '.')
    raise 'dupl reported clone groups; review and adapt the explicit project check' unless out.strip.empty? ||
                                                                                           out.strip == 'Found total 0 clone groups.'
  end

  def gate(full)
    %w[go gofmt staticcheck go-audit].each { |name| executable(name) }
    DEEP_TOOLS.each { |name| executable(name) } if full
    checked = input_digest
    out = run(executable('gofmt'), '-s', '-l', '.')
    raise "formatting required:\n#{out}" unless out.strip.empty?

    run(executable('go'), 'mod', 'tidy', '-diff')
    script('lint')
    ToolSettings::FAST_CHECKS.each { |args| run(*args) }
    run(executable('go'), 'test', '-timeout', ToolSettings::TEST_TIMEOUT, './...')
    if full
      run(executable('go'), 'test', '-race', '-timeout', ToolSettings::TEST_TIMEOUT, './...')
      deep_review
      ToolSettings::FULL_CHECKS.each { |args| run(*args) }
    end
    script('build')
    raise 'gate changed source or verification inputs; inspect changes and rerun' unless checked == input_digest

    puts "#{full ? 'Full' : 'Fast'} gate passed"
  end

  def state_path(name)
    git('rev-parse', '--git-path', "tooling-#{name}.json").strip
  end

  def head
    git('rev-parse', 'HEAD').strip
  end

  def input_digest(exclude_version: false)
    files = git('ls-files', '-c', '-o', '--exclude-standard', '-z').split("\0")
    files.select! { |path| File.exist?(path) || File.symlink?(path) }
    files.concat(Dir.glob('tools/**/*', File::FNM_DOTMATCH).select { |path| File.file?(path) })
    files.push('go.mod', 'go.sum', ToolSettings::VERSION_FILE)
    files.concat(ToolSettings::VERIFICATION_INPUTS.flat_map { |pattern| Dir.glob(pattern, File::FNM_DOTMATCH) })
    files -= [ToolSettings::VERSION_FILE] if exclude_version
    digest = Digest::SHA256.new
    files.uniq.sort.each do |path|
      digest << file_input(path)
    end
    digest << run_environment
    digest.hexdigest
  end

  def file_input(path)
    return "#{path}\0missing\0" unless File.exist?(path)

    resolved = File.symlink?(path) ? File.realpath(path) : path
    "#{path}\0#{resolved}\0#{File.binread(path)}\0#{File.stat(path).mode & 0o777}\0"
  end

  def run_environment
    goenv, err, status = capture(executable('go'), 'env', '-json')
    raise "go env failed: #{err}" unless status.success?

    tools = (%w[go gofmt staticcheck go-audit] + DEEP_TOOLS).map do |name|
      path = ToolSettings::TOOLS[name] ||
             search_paths.map { |dir| File.join(dir, name) }.find { |file| File.file?(file) && File.executable?(file) }
      [name, path, path && File.file?(path) ? Digest::SHA256.file(path).hexdigest : nil]
    end
    configuration = JSON.parse(goenv)
    configuration.delete('GOGCCFLAGS') # Go includes a different temporary build directory on each query.
    JSON.generate([configuration, tools, ENV.to_h.select { |key, _| key.start_with?('GO', 'CGO', 'CC', 'CXX') }])
  end

  def record(level)
    data = { head: head, level: level, inputs: input_digest }
    data[:metadata_inputs] = input_digest(exclude_version: true) if metadata_version?
    File.write(state_path('verification'), JSON.generate(data))
  end

  def verified?(metadata: false)
    path = state_path('verification')
    return false unless File.file?(path)

    data = JSON.parse(File.read(path), symbolize_names: true)
    return false unless data.is_a?(Hash)

    key = metadata ? :metadata_inputs : :inputs
    data[:head] == head && data[:level] == 'full' && data[key] == input_digest(exclude_version: metadata)
  rescue JSON::ParserError
    false
  end

  def metadata_version?
    return false unless ToolSettings::VERSION_KIND == :metadata
    return false unless ToolSettings::VERSION_UPDATE.empty?

    format = '{{.Dir}}|{{join .EmbedFiles ","}},{{join .TestEmbedFiles ","}},{{join .XTestEmbedFiles ","}}'
    out = run(executable('go'), 'list', '-f', format, './...')
    version = File.expand_path(ToolSettings::VERSION_FILE)
    out.lines.none? do |line|
      dir, paths = line.strip.split('|', 2)
      paths.to_s.split(',').any? { |path| File.expand_path(path, dir) == version }
    end
  end

  def commit(message, full)
    script('gate', *('--full' if full))
    checked = input_digest
    git('add', '-A')
    git('commit', '-m', message)
    raise 'commit hook changed checked inputs; run the gate again before releasing' unless checked == input_digest

    record(full ? 'full' : 'fast')
    puts "Committed: #{message}"
  end

  def upstream
    branch = git('symbolic-ref', '--short', 'HEAD').strip
    remote = git('config', '--get', "branch.#{branch}.remote").strip
    ref = git('config', '--get', "branch.#{branch}.merge").strip
    raise 'release requires a remote branch upstream' if remote.empty? || remote == '.' || !ref.start_with?('refs/heads/')

    git('check-ref-format', ref)
    destinations = git('remote', 'get-url', '--push', '--all', remote).lines.map(&:strip)
    raise 'release requires exactly one upstream push destination' unless destinations.length == 1

    [destinations.first, ref]
  end

  def version
    path = contained_path(ToolSettings::VERSION_FILE)
    raise "versioned releases unsupported: missing #{path}" unless File.file?(path)

    value = File.read(path).strip
    raise "invalid semantic version: #{value.inspect}" unless VERSION.match?(value)

    [path, value]
  end

  def bump
    raise 'unfinished release; use tools/bump --resume after inspecting local state' if File.exist?(state_path('release'))

    remote, ref = upstream
    path, current = version
    raise 'bump requires a clean working tree; commit or snapshot first' unless git('status', '--porcelain').empty?

    reuse = metadata_version? && verified?(metadata: true)
    updated = current.split('.').tap { |parts| parts[-1] = (parts[-1].to_i + 1).to_s }.join('.')
    File.write(state_path('release'), JSON.generate(base: head, version: updated, remote: remote, ref: ref))
    File.write(path, "#{updated}\n")
    ToolSettings::VERSION_UPDATE.each { |args| run(*args) }
    script(reuse ? 'build' : 'gate', *('--full' unless reuse))
    checked = input_digest
    git('add', '-A')
    git('commit', '-m', "chore: bump version to #{updated}")
    raise 'commit hook changed checked release inputs; inspect and recheck before publishing' unless checked == input_digest

    data = { base: nil, commit: head, version: updated, remote: remote, ref: ref }
    File.write(state_path('release'), JSON.generate(data))
    push_release(data)
  rescue StandardError => e
    raise "#{e.message}\nLocal state retained. Inspect changes; tools/bump --resume never increments the version."
  end

  def resume
    path = state_path('release')
    raise 'no interrupted release to resume' unless File.file?(path)

    data = JSON.parse(File.read(path), symbolize_names: true)
    data = complete_pending_commit(data) unless data[:commit]
    raise 'HEAD or working tree changed since release commit' unless head == data.fetch(:commit) && git('status', '--porcelain').empty?

    push_release(data)
  end

  def complete_pending_commit(data)
    raise 'HEAD changed before pending release commit; inspect local history' unless head == data.fetch(:base)
    raise 'pending version changed; inspect local files' unless version.last == data.fetch(:version)

    script('gate', '--full')
    checked = input_digest
    git('add', '-A')
    git('commit', '-m', "chore: bump version to #{data.fetch(:version)}")
    raise 'commit hook changed checked inputs; inspect local state' unless checked == input_digest

    data[:commit] = head
    File.write(state_path('release'), JSON.generate(data))
    data
  end

  def push_release(data)
    sha, remote, ref = data.values_at(:commit, :remote, :ref)
    git('push', remote, "#{sha}:#{ref}")
    actual = git('ls-remote', '--exit-code', remote, ref).split.first
    raise "remote verification mismatch: expected #{sha}, got #{actual.inspect}" unless actual == sha

    FileUtils.rm_f(state_path('verification'))
    FileUtils.rm_f(state_path('release'))
    puts "Released #{data.fetch(:version)}: #{sha} pushed and verified"
  end

  def install
    targets = ToolSettings::INSTALL_TARGETS
    raise 'installation unsupported: declare INSTALL_TARGETS in tools/settings.rb' if targets.empty?

    sources = artifacts
    targets.each_key { |source| raise "installation source is not a declared artifact: #{source}" unless sources.key?(source) }
    script('build')
    targets.each do |source, destination|
      built = contained_path(source)
      raise "installation target must be absolute: #{destination}" unless destination.start_with?('/')
      raise "installation target cannot be a symlink: #{destination}" if File.symlink?(destination)
      raise "installation target cannot be a directory: #{destination}" if File.directory?(destination)

      replace_file(built, destination)
      raise "installed artifact mismatch: #{destination}" unless Digest::SHA256.file(built) == Digest::SHA256.file(destination)
      raise "installed permissions mismatch: #{destination}" unless File.stat(built).mode == File.stat(destination).mode
    end
    puts 'Installation verified'
  end

  # Copy beside the destination, then rename over it. Writing into the
  # destination itself fails with ETXTBSY while the installed binary is running
  # (an open TUI or `watch`); a rename swaps the directory entry instead, so the
  # running process keeps the old file and the next launch gets the new one. It
  # also means an interrupted install never leaves a half-written binary.
  def replace_file(source, destination)
    FileUtils.mkdir_p(File.dirname(destination))
    staged = "#{destination}.installing-#{Process.pid}"
    FileUtils.cp(source, staged, preserve: true)
    File.rename(staged, destination)
  ensure
    FileUtils.rm_f(staged) if staged
  end

  def release(message, install_requested)
    published = false
    upstream
    version
    script('commit', message)
    script('bump')
    published = true
    return unless install_requested

    script('install')
  rescue StandardError => e
    state = if published
              'Release published and verified; installation failed. Retry tools/install separately.'
            else
              'Release did not complete; the message above says which step failed and what, if anything, was published.'
            end
    raise "#{e.message}\n#{state}"
  end

  def entry
    setup
    yield
  rescue StandardError => e
    abort e.message
  end
end
