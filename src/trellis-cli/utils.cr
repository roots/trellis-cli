require "levenshtein"

module Trellis::CLI
  module Utils
    def run_command(command, output = true)
      puts "Running => #{command}" if output

      output = output ? STDOUT : Process::Redirect::Close
      Process.run(command, shell: true, output: output, error: STDOUT)
    end
  end
end
