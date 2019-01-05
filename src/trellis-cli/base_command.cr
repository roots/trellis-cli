require "yaml"

module Trellis::CLI
  abstract class BaseCommand < Cli::Command
    class WordpressSites
      YAML.mapping(
        wordpress_sites: Hash(String, Hash(String, YAML::Any))
      )
    end

    private def fetch_environments
      Dir.cd("group_vars") do
        envs = Dir.glob("*/wordpress_sites.yml").map do |config|
          File.dirname(config)
        end

        envs.sort
      end
    end

    private def fetch_sites(environment : String)
      sites = WordpressSites.from_yaml(File.read("group_vars/#{environment}/wordpress_sites.yml")).wordpress_sites
      sites.keys
    end

    private def validate_environment(env)
      environments = fetch_environments
      return env if environments.includes?(env)

      if suggestion = Levenshtein.find(env, environments, tolerance: 2)
        print "#{env} not a valid environment. Did you mean #{suggestion}? (Y/n) "
        STDOUT.flush

        if gets(limit: 1) == "Y"
          suggestion
        else
          error!
        end
      else
        error! "#{env} not a valid environment. Environments: #{environments.join(", ")}"
      end
    end

    private def validate_site(environment : String, site : String)
      sites = fetch_sites(environment)
      return site if sites.includes?(site)

      if suggestion = Levenshtein.find(site, sites, tolerance: 3)
        print "#{site} not a valid a site. Did you mean #{suggestion}? (Y/n) "
        STDOUT.flush

        if gets(limit: 1, chomp: false) == "Y"
          suggestion
        else
          error!
        end
      else
        error! "#{site} not a valid site. Sites: #{sites.join(", ")}"
      end
    end
  end
end
