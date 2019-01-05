require "file_utils"
require "http/client"
require "json"
require "../utils"

module Trellis::CLI
  module Command
    class New
      include Utils

      def initialize(@name : String, path : String)
        @original_path = path
        @path = File.expand_path(@name, @original_path)
      end

      def run
        puts "Creating new Trellis project in #{@original_path}"
        puts
        puts "Fetching latest versions of Trellis and Bedrock..."

        FileUtils.mkdir_p(@path) unless File.exists?(@path)
        FileUtils.cd(@path)

        trellis_version = download_latest_release(repo: "roots/trellis", dest: File.join(@path, "trellis"))
        bedrock_version = download_latest_release(repo: "roots/bedrock", dest: File.join(@path, "site"))

        puts
        puts "#{@name} project created with:"
        puts "  Trellis v#{trellis_version}"
        puts "  Bedrock v#{bedrock_version}"
      end

      private def download_latest_release(repo : String, dest : String)
        release = fetch_latest_release(repo)

        zip_url = release["zipball_url"].as_s
        version = release["tag_name"].as_s

        run_command("wget #{zip_url}", output: false)
        run_command("unzip #{version}", output: false)

        Dir["roots-*"].each do |dir|
          FileUtils.mv(dir, dest)
        end

        FileUtils.rm(version)

        version
      end

      private def fetch_latest_release(repo)
        url = "https://api.github.com/repos/#{repo}/releases/latest"
        response = HTTP::Client.get(url)

        JSON.parse(response.body)
      end
    end
  end
end
