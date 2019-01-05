require "cli"
require "./command/*"
require "./base_command"
require "./utils"

module Trellis::CLI
  class App < Cli::Supercommand
    command_name "trellis"

    command "help", default: true
    version VERSION

    class Options
      help
      version
    end

    class Help
      header "Trellis CLI"
      footer "(C) #{Time.now.year} Roots"
    end

    class Provision < BaseCommand
      include Utils

      class Help
        caption "Provisions a server for the specified environment"
      end

      class Options
        arg "environment", required: true, desc: "Name of environment (ie: `production`)", complete: "trellis info --only-environments"
        array %w(-t --tags)
        help
      end

      def run
        environment = validate_environment(args.environment)

        command = "ansible-playbook server.yml"
        command = command + %Q( -e "env=#{environment}")
        command = command + " --tags=#{args.tags.join(',')}" unless args.tags.empty?

        run_command(command)
      end
    end

    class Deploy < BaseCommand
      include Utils

      class Help
        caption "Deploys a single site to an environment"
      end

      class Options
        arg "environment", required: true, desc: "Name of environment (ie: `production`)", complete: "trellis info --only-environments"
        arg "site", required: true, desc: "Name of site to deploy (ie: `example.com`)", complete: "trellis info --environment ${COMP_WORDS[2]}"
        help
      end

      def run
        environment = validate_environment(args.environment)
        site = validate_site(environment, args.site)

        run_command("./bin/deploy.sh #{environment} #{site}")
      end
    end

    class Rollback < BaseCommand
      include Utils

      class Help
        caption "Rolls back a deploy on a single site"
      end

      class Options
        arg "environment", required: true, desc: "Name of environment (ie: `production`)", complete: "trellis info --only-environments"
        arg "site", required: true, desc: "Name of site to rollback (ie: `example.com`)", complete: "trellis info --environment ${COMP_WORDS[2]}"
        help
      end

      def run
        environment = validate_environment(args.environment)
        site = validate_site(environment, args.site)

        command = "ansible-playbook rollback.yml"
        command = command + %Q( -e "env=#{environment} site=#{site}")

        run_command(command)
      end
    end

    class New < BaseCommand
      class Options
        arg "name", required: true, desc: "Name of new Trellis site (ie: `example.com`)"
        arg "path", default: ".", desc: "Path to create new project in (defaults to current directory)", complete: :directory
        help
      end

      class Help
        caption "Creates a new Trellis project"
      end

      def run
        Command::New.new(args.name, args.path).run
      end
    end

    class Info < BaseCommand
      include Utils

      class Help
        caption "Displays information about this Trellis project"
      end

      class Options
        bool "--only-environments"
        string "--environment"

        help
      end

      def run
        environments = fetch_environments

        if options.only_environments?
          environments.each do |env|
            puts env
          end
        elsif options.environment
          fetch_sites(options.environment).each do |site|
            puts site
          end
        else
          fetch_environments.each do |environment|
            sites = fetch_sites(environment)

            puts "#{environment} => #{sites.join(", ")}"
          end
        end
      end
    end

    class Completions < BaseCommand
      class Help
        caption "Generate shell completions"
      end

      class Options
        arg "shell", default: "bash", any_of: %w(bash zsh), desc: "Type of shell to generate completions for"

        help
      end

      def run
        case args.shell
        when "bash"
          puts App.generate_bash_completion
        when "zsh"
          puts App.generate_zsh_completion
        end
      end
    end
  end
end
