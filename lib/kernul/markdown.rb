require "redcarpet"
require "net/http"

module Kernul
  class PygmentsMarkdownRenderer < Redcarpet::Render::HTML
    PYGMENTIZE_URL = URI.parse('http://pygmentize.herokuapp.com/')

    def block_code(code, language)
      Net::HTTP.post_form(PYGMENTIZE_URL, code: code, lang: language).body
    end
  end

  MARKDOWN = Redcarpet::Markdown.new(PygmentsMarkdownRenderer, fenced_code_blocks: true)
end
