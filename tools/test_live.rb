#!/usr/bin/env ruby
# frozen_string_literal: true

require 'json'
require 'optparse'
require 'open3'

# Terminal ANSI color helpers
module Colors
  GREEN = "\e[32m"
  RED = "\e[31m"
  YELLOW = "\e[33m"
  CYAN = "\e[36m"
  BOLD = "\e[1m"
  RESET = "\e[0m"

  def self.success(str) = "#{GREEN}✔ #{str}#{RESET}"
  def self.fail(str)    = "#{RED}✖ #{str}#{RESET}"
  def self.warn(str)    = "#{YELLOW}⚠ #{str}#{RESET}"
  def self.info(str)    = "#{CYAN}ℹ #{str}#{RESET}"
end

def load_config
  config_path = File.expand_path('~/.config/mail_cli/config.json')
  return nil unless File.exist?(config_path)

  JSON.parse(File.read(config_path))
rescue StandardError => e
  warn Colors.warn("Failed to parse config: #{e.message}")
  nil
end

def find_test_accounts(config)
  return [] unless config && config['accounts'].is_a?(Array)

  config['accounts'].select do |acc|
    acc['account_type'] == 'test' || acc['name'].to_s.end_with?('_test')
  end
end

def run_suite(name, env, test_pattern, verbose)
  puts "\n#{Colors::BOLD}=== Running #{name} ===#{Colors::RESET}"
  cmd = ['go', 'test', '-count=1', './integration/', '-run', test_pattern]
  cmd << '-v' if verbose

  start_time = Time.now
  stdout, stderr, status = Open3.capture3(env, *cmd)
  elapsed = (Time.now - start_time).round(2)

  if status.success?
    puts stdout if verbose
    puts Colors.success("#{name} passed in #{elapsed}s")
    true
  else
    puts stderr unless stderr.empty?
    puts stdout
    puts Colors.fail("#{name} failed with exit code #{status.exitstatus} (#{elapsed}s)")
    false
  end
end

def main
  options = {
    readonly: false,
    mutate: false,
    cli: false,
    all: false,
    account: nil,
    verbose: true
  }

  OptionParser.new do |opts|
    opts.banner = 'Usage: tools/test_live.rb [options]'

    opts.on('-r', '--readonly', 'Run read-only live tests against all configured accounts') do
      options[:readonly] = true
    end

    opts.on('-m', '--mutate', 'Run mutating live tests against the designated test account') do
      options[:mutate] = true
    end

    opts.on('-c', '--cli', 'Run black-box CLI live tests against the designated test account') do
      options[:cli] = true
    end

    opts.on('-a', '--all', 'Run all live test suites (readonly, mutate, cli)') do
      options[:all] = true
    end

    opts.on('-A', '--account NAME', 'Specify target test account name') do |v|
      options[:account] = v
    end

    opts.on('-q', '--quiet', 'Suppress verbose test output') do
      options[:verbose] = false
    end

    opts.on('-h', '--help', 'Show this help message') do
      puts opts
      exit 0
    end
  end.parse!

  # Default to running all suites if no specific suite selected
  options[:all] = true unless options[:readonly] || options[:mutate] || options[:cli]

  config = load_config
  test_accounts = find_test_accounts(config)

  target_account = options[:account]
  if target_account.nil?
    if test_accounts.empty?
      puts Colors.warn('No test account configured with account_type "test".')
      puts "To configure one, run: mail_cli account new <type> <name> --test\n"
    else
      target_account = test_accounts.first['name']
    end
  end

  puts "#{Colors::BOLD}mail_cli Live Test Suite Runner#{Colors::RESET}"
  puts "Test Account: #{target_account ? "#{Colors::CYAN}#{target_account}#{Colors::RESET}" : "#{Colors::YELLOW}[none]#{Colors::RESET}"}"

  failures = 0
  passes = 0

  # 1. Read-Only Suite
  if options[:all] || options[:readonly]
    env = { 'MAIL_CLI_LIVE' => '1' }
    if run_suite('Read-Only Live Suite', env, 'Live', options[:verbose])
      passes += 1
    else
      failures += 1
    end
  end

  # 2. Mutating Suite
  if (options[:all] || options[:mutate]) && target_account
    env = {
      'MAIL_CLI_LIVE_MUTATE' => '1',
      'MAIL_CLI_MUTATE_ACCOUNT' => target_account
    }
    if run_suite("Mutating Live Suite (#{target_account})", env, 'Mutation', options[:verbose])
      passes += 1
    else
      failures += 1
    end
  end

  # 3. CLI Black-Box Suite
  if (options[:all] || options[:cli]) && target_account
    env = {
      'MAIL_CLI_LIVE_MUTATE' => '1',
      'MAIL_CLI_MUTATE_ACCOUNT' => target_account
    }
    if run_suite("CLI Black-Box Live Suite (#{target_account})", env, 'TestCLI', options[:verbose])
      passes += 1
    else
      failures += 1
    end
  end

  puts "\n#{Colors::BOLD}=== Live Test Summary ===#{Colors::RESET}"
  puts "Suites Passed: #{Colors::GREEN}#{passes}#{Colors::RESET}"
  puts "Suites Failed: #{failures.zero? ? '0' : "#{Colors::RED}#{failures}#{Colors::RESET}"}"

  exit(failures.zero? ? 0 : 1)
end

main if __FILE__ == $PROGRAM_NAME
