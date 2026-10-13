#!/usr/bin/env ruby
# frozen_string_literal: true

# Converts Mutt alias files into a Google Contacts CSV import file.
# Resolves nested groups, merges multi-email contacts, maps aliases to Nicknames,
# and maps distribution lists to Google Contact Labels.

require 'csv'
require 'optparse'
require 'set'

class MuttAliasParser
  Entry = Struct.new(:key, :raw_value, :recipients, :is_group, keyword_init: true)
  Recipient = Struct.new(:name, :email, :alias_ref, keyword_init: true)

  attr_reader :entries

  def initialize(filepath)
    @filepath = filepath
    @entries = {}
    parse_file
  end

  def parse_file
    raw_aliases = {}
    current_key = nil
    current_lines = []

    File.readlines(@filepath).each do |line|
      # Strip inline comments (e.g. # comment)
      s = line.sub(/#.*$/, '').strip
      next if s.empty?

      if s =~ /^\s*alias\s+(\S+)\s+(.*)$/
        raw_aliases[current_key] = current_lines.join(' ') if current_key
        current_key = Regexp.last_match(1)
        current_lines = [Regexp.last_match(2)]
      elsif current_key
        current_lines << s
      end
    end
    raw_aliases[current_key] = current_lines.join(' ') if current_key

    raw_aliases.each do |key, val|
      recipients = parse_value(val)
      is_group = recipients.size > 1 || (recipients.size == 1 && recipients.first.alias_ref)
      @entries[key] = Entry.new(
        key: key,
        raw_value: val,
        recipients: recipients,
        is_group: is_group
      )
    end
  end

  def parse_value(str)
    clean = str.gsub(/\\\s*/, ' ').strip
    tokens = split_by_commas(clean)

    if tokens.size == 1 && !tokens.first.include?('<') && !tokens.first.include?('@')
      sub_tokens = tokens.first.split(/\s+/)
      tokens = sub_tokens if sub_tokens.size > 1
    end

    tokens.map { |tok| parse_single_recipient(tok.strip) }.compact
  end

  def split_by_commas(str)
    parts = []
    current = String.new
    in_quote = false
    escape = false

    str.each_char do |ch|
      if escape
        current << ch
        escape = false
      elsif ch == '\\'
        escape = true
      elsif ch == '"'
        in_quote = !in_quote
        current << ch
      elsif ch == ',' && !in_quote
        parts << current.strip unless current.strip.empty?
        current = String.new
      else
        current << ch
      end
    end
    parts << current.strip unless current.strip.empty?
    parts
  end

  def parse_single_recipient(tok)
    return nil if tok.empty?

    if tok =~ /^(.*?)\s*<([^>]+)>$/
      raw_name = Regexp.last_match(1).strip
      email = Regexp.last_match(2).strip
      name = raw_name.sub(/^["']/, '').sub(/["']$/, '').strip
      Recipient.new(name: name, email: email, alias_ref: nil)
    elsif tok =~ /^[^\s<>@]+@[^\s<>@]+$/
      Recipient.new(name: '', email: tok, alias_ref: nil)
    elsif tok =~ /^\S+$/ && !tok.include?('@')
      Recipient.new(name: '', email: nil, alias_ref: tok)
    else
      Recipient.new(name: tok, email: nil, alias_ref: tok)
    end
  end

  def flatten_all
    flattened = {}
    @entries.each_key do |key|
      flattened[key] = resolve_recipients(key, Set.new)
    end
    flattened
  end

  def resolve_recipients(key, visited)
    return [] if visited.include?(key)

    visited.add(key)
    entry = @entries[key]
    return [] unless entry

    resolved = []
    entry.recipients.each do |rec|
      if rec.alias_ref
        nested = resolve_recipients(rec.alias_ref, visited.dup)
        resolved.concat(nested)
      else
        resolved << rec
      end
    end

    resolved.uniq { |r| r.email&.downcase }
  end
end

class GoogleContactsExporter
  def initialize(flattened, options = {})
    @flattened = flattened
    @group_prefix = options[:group_prefix] || ''
  end

  def consolidate
    email_to_contact = {}
    name_to_contact = {}
    contacts = []

    # Sort keys so non-old / primary aliases process before _old/_older suffixes
    sorted_keys = @flattened.keys.sort_by do |k|
      is_group = @flattened[k].size > 1 ? 1 : 0
      is_old = k =~ /_old/ ? 1 : 0
      [is_group, is_old, k]
    end

    # Pass 1: Individual aliases
    sorted_keys.each do |key|
      recipients = @flattened[key]
      next if recipients.size > 1

      rec = recipients.first
      next unless rec&.email

      email = rec.email.strip.downcase
      name = rec.name.to_s.strip
      norm_name = name.downcase.gsub(/[^a-z0-9]/, ' ').gsub(/\s+/, ' ').strip

      contact = email_to_contact[email]
      if contact.nil? && !norm_name.empty?
        contact = name_to_contact[norm_name]
      end

      if contact.nil?
        parts = name.split(/\s+/)
        given_name = parts.first || ''
        family_name = parts.size > 1 ? parts[1..].join(' ') : ''

        contact = {
          display_name: name.empty? ? key : name,
          given_name: given_name,
          family_name: family_name,
          emails: [],
          nicknames: Set.new,
          groups: Set.new,
          notes: []
        }
        contacts << contact
        name_to_contact[norm_name] = contact unless norm_name.empty?
      end

      contact[:nicknames].add(key)
      contact[:emails] << email unless contact[:emails].include?(email)
      email_to_contact[email] = contact
    end

    # Pass 2: Groups (distribution lists)
    @flattened.each do |group_name, recipients|
      next if recipients.size <= 1

      label = "#{@group_prefix}#{group_name}"

      recipients.each do |rec|
        next unless rec&.email

        email = rec.email.strip.downcase
        contact = email_to_contact[email]
        if contact.nil?
          name = rec.name.to_s.strip
          parts = name.split(/\s+/)
          given_name = parts.first || ''
          family_name = parts.size > 1 ? parts[1..].join(' ') : ''
          contact = {
            display_name: name.empty? ? email : name,
            given_name: given_name,
            family_name: family_name,
            emails: [email],
            nicknames: Set.new,
            groups: Set.new,
            notes: []
          }
          contacts << contact
          email_to_contact[email] = contact
        end
        contact[:groups].add(label)
      end
    end

    contacts
  end

  def export_csv(output_path)
    contacts = consolidate

    headers = [
      'Name', 'Given Name', 'Family Name', 'Additional Name', 'Nickname',
      'Notes',
      'E-mail 1 - Type', 'E-mail 1 - Value',
      'E-mail 2 - Type', 'E-mail 2 - Value',
      'E-mail 3 - Type', 'E-mail 3 - Value',
      'E-mail 4 - Type', 'E-mail 4 - Value',
      'Group Membership'
    ]

    CSV.open(output_path, 'w', write_headers: true, headers: headers) do |csv|
      contacts.each do |c|
        nicknames = c[:nicknames].to_a.join(', ')
        groups = (['* myContacts'] + c[:groups].to_a).join(' ::: ')
        notes = c[:nicknames].any? ? "Mutt aliases: #{nicknames}" : ''

        row = [
          c[:display_name],
          c[:given_name],
          c[:family_name],
          '', # Additional Name
          nicknames,
          notes
        ]

        # Add up to 4 emails
        4.times do |i|
          if c[:emails][i]
            row << (i.zero? ? 'Work' : 'Other')
            row << c[:emails][i]
          else
            row << ''
            row << ''
          end
        end

        row << groups
        csv << row
      end
    end

    contacts
  end
end

options = {
  input: 'tmp/mail_aliases',
  output: 'tmp/google_contacts.csv',
  group_prefix: ''
}

OptionParser.new do |opts|
  opts.banner = 'Usage: tools/mutt_to_google_contacts.rb [options]'
  opts.on('-i', '--input FILE', 'Input mutt aliases file (default: tmp/mail_aliases)') { |v| options[:input] = v }
  opts.on('-o', '--output FILE', 'Output CSV file (default: tmp/google_contacts.csv)') { |v| options[:output] = v }
  opts.on('-p', '--prefix PREFIX', 'Optional prefix for group labels (e.g. mutt_)') { |v| options[:group_prefix] = v }
end.parse!

unless File.exist?(options[:input])
  warn "Error: Input file '#{options[:input]}' not found."
  exit 1
end

parser = MuttAliasParser.new(options[:input])
flattened = parser.flatten_all
exporter = GoogleContactsExporter.new(flattened, options)
contacts = exporter.export_csv(options[:output])

puts "Successfully generated Google Contacts CSV: #{options[:output]}"
puts "  Total raw aliases:      #{parser.entries.size}"
puts "  Consolidated contacts:  #{contacts.size}"
puts "  Merged multi-emails:    #{contacts.count { |c| c[:emails].size > 1 }}"
puts "  Group members mapped:   #{contacts.count { |c| c[:groups].any? }}"
puts "  Nicknames populated:    #{contacts.count { |c| c[:nicknames].any? }}"
