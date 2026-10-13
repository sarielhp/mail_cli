#!/usr/bin/env ruby
# frozen_string_literal: true

require 'find'
require 'open3'
require 'optparse'

options = {
  ext: nil,
  quiet: false
}

OptionParser.new do |opts|
  opts.banner = "Usage: #{$PROGRAM_NAME} [options] [directory_or_file...]"

  opts.on('-e', '--ext EXT', 'Filter by extension (e.g. .mbox)') do |v|
    ext = v.start_with?('.') ? v : ".#{v}"
    options[:ext] = ext
  end

  opts.on('-q', '--quiet', 'Only print invalid files and final summary') do
    options[:quiet] = true
  end

  opts.on('-h', '--help', 'Show this help message') do
    puts opts
    exit 0
  end
end.parse!

targets = ARGV.empty? ? ['.'] : ARGV

use_color = $stdout.tty?
c_red   = use_color ? "\e[31m" : ''
c_green = use_color ? "\e[32m" : ''
c_reset = use_color ? "\e[0m"  : ''

valid_count = 0
invalid_count = 0

targets.each do |target|
  unless File.exist?(target)
    warn "#{c_red}Target does not exist:#{c_reset} #{target}"
    invalid_count += 1
    next
  end

  Find.find(target) do |path|
    next unless File.file?(path)
    next if options[:ext] && File.extname(path) != options[:ext]

    stdout, stderr, status = Open3.capture3('frm', '-Q', '-S', path)

    # In GNU frm:
    # exit 3 = could not open mailbox / invalid format
    # exit 0, 1, 2 = valid mailbox (status of unread/read messages)
    if status.exitstatus == 3
      invalid_count += 1
      err_msg = stderr.strip.empty? ? 'Not a valid mailbox' : stderr.strip
      warn "#{c_red}[INVALID]#{c_reset} #{path} — #{err_msg}"
    else
      valid_count += 1
      unless options[:quiet]
        info = stdout.strip
        detail = info.empty? ? 'valid mailbox' : info
        puts "#{c_green}[VALID]#{c_reset}   #{path} (#{detail})"
      end
    end
  end
end

puts "\nFinished: #{valid_count} valid, #{invalid_count} invalid."
exit(invalid_count.zero? ? 0 : 1)
