#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

Tooling.entry do
  abort 'Usage: tools/gate [--full]' unless ARGV.empty? || ARGV == ['--full']
  Tooling.gate(ARGV == ['--full'])
end
