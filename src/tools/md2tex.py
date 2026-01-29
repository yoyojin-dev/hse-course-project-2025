import argparse
import re
from pathlib import Path
from typing import List


def escape_latex(text: str) -> str:
    # Keep it conservative: escape common specials.
    # Do NOT escape backslashes (already LaTeX) and do not touch braces.
    replacements = {
        "&": r"\&",
        "%": r"\%",
        "$": r"\$",
        "#": r"\#",
        "_": r"\_",
        "{": r"\{",
        "}": r"\}",
        "~": r"\textasciitilde{}",
        "^": r"\textasciicircum{}",
    }
    out = []
    for ch in text:
        out.append(replacements.get(ch, ch))
    return "".join(out)


def md_inline_to_tex(s: str) -> str:
    # Escape first, then re-introduce structures carefully.
    s = escape_latex(s)

    # Inline code: `code`
    s = re.sub(r"`([^`]+)`", lambda m: r"\texttt{" + escape_latex(m.group(1)) + "}", s)

    # Links: [text](url)
    def _link(m: re.Match) -> str:
        label = m.group(1)
        url = m.group(2)
        return r"\href{" + url + "}{" + label + "}"

    s = re.sub(r"\[([^\]]+)\]\(([^)]+)\)", _link, s)

    # Bold: **text**
    s = re.sub(r"\*\*([^*]+)\*\*", r"\\textbf{\1}", s)

    # Italic: *text* (simple; avoids underscores already escaped)
    s = re.sub(r"(?<!\*)\*([^*]+)\*(?!\*)", r"\\textit{\1}", s)

    return s


def fence_language_to_listings(lang: str) -> str:
    lang = (lang or "").strip().lower()
    mapping = {
        "python": "Python",
        "py": "Python",
        "bash": "bash",
        "sh": "bash",
        "json": "json",
        "yaml": "yaml",
        "yml": "yaml",
        "sql": "SQL",
        "tex": "TeX",
        "latex": "TeX",
        "text": "",
        "": "",
    }
    return mapping.get(lang, lang)


def md_to_tex(md_text: str) -> str:
    lines = md_text.splitlines()
    out: List[str] = []

    in_code = False
    in_quote = False

    # стек уровней списков: [{"type": "ul"/"ol", "indent": int}, ...]
    list_stack: List[dict] = []

    def close_all_lists() -> None:
        nonlocal list_stack
        while list_stack:
            level = list_stack.pop()
            out.append(r"\end{itemize}" if level["type"] == "ul" else r"\end{enumerate}")

    def close_quote() -> None:
        nonlocal in_quote
        if in_quote:
            out.append(r"\end{quote}")
            in_quote = False

    fence_re = re.compile(r"^```(\w+)?\s*$")
    heading_re = re.compile(r"^(#{1,6})\s+(.*)$")
    # сохраняем отступ и тип
    ul_re = re.compile(r"^(?P<indent>\s*)[-*+]\s+(?P<text>.*)$")
    ol_re = re.compile(r"^(?P<indent>\s*)(?P<num>\d+)\.\s+(?P<text>.*)$")
    img_re = re.compile(r"^!\[([^\]]*)\]\(([^)]+)\)\s*$")
    hr_re = re.compile(r"^\s*(---|\*\*\*|___)\s*$")
    quote_re = re.compile(r"^\s*>\s?(.*)$")

    # --- helper: заглядывание вперёд ---
    def next_nonempty_index(start_idx: int) -> int | None:
        """Вернуть индекс следующей непустой строки или None."""
        for j in range(start_idx + 1, len(lines)):
            if lines[j].strip():
                return j
        return None

    i = 0
    while i < len(lines):
        raw = lines[i]
        line = raw.rstrip("\n")

        # Code fences
        m = fence_re.match(line)
        if m:
            if not in_code:
                close_all_lists()
                close_quote()
                in_code = True
                code_lang = fence_language_to_listings(m.group(1) or "")
                if code_lang:
                    out.append(r"\begin{lstlisting}[language=" + code_lang + "]")
                else:
                    out.append(r"\begin{lstlisting}")
            else:
                out.append(r"\end{lstlisting}")
                in_code = False
            i += 1
            continue

        if in_code:
            out.append(line.replace("\t", "    "))
            i += 1
            continue

        # Horizontal rule
        if hr_re.match(line):
            close_all_lists()
            close_quote()
            out.append(r"\hrule")
            out.append("")
            i += 1
            continue

        # Blockquote
        qm = quote_re.match(line)
        if qm:
            close_all_lists()
            if not in_quote:
                out.append(r"\begin{quote}")
                in_quote = True
            out.append(md_inline_to_tex(qm.group(1)))
            i += 1
            continue
        else:
            close_quote()

        # Image
        im = img_re.match(line)
        if im:
            close_all_lists()
            alt = md_inline_to_tex(im.group(1).strip())
            path = im.group(2).strip()
            out.append(r"\begin{figure}[h]")
            out.append(r"\centering")
            out.append(r"\includegraphics[width=\linewidth]{" + path + "}")
            if alt:
                out.append(r"\caption{" + alt + "}")
            out.append(r"\end{figure}")
            out.append("")
            i += 1
            continue

        # Headings
        hm = heading_re.match(line)
        if hm:
            close_all_lists()
            level = len(hm.group(1))
            title = md_inline_to_tex(hm.group(2).strip())
            if level == 1:
                out.append(r"\section{" + title + "}")
            elif level == 2:
                out.append(r"\subsection{" + title + "}")
            elif level == 3:
                out.append(r"\subsubsection{" + title + "}")
            else:
                out.append(r"\paragraph{" + title + "}")
            out.append("")
            i += 1
            continue

        # Ordered list
        om = ol_re.match(line)
        if om:
            indent = len(om.group("indent") or "")
            text = om.group("text").strip()
            # выравниваем стек по отступу
            while list_stack and indent < list_stack[-1]["indent"]:
                level = list_stack.pop()
                out.append(r"\end{itemize}" if level["type"] == "ul" else r"\end{enumerate}")
            current_type = "ol"
            if not list_stack or indent > list_stack[-1]["indent"]:
                # новый вложенный уровень
                out.append(r"\begin{enumerate}")
                list_stack.append({"type": current_type, "indent": indent})
            elif list_stack[-1]["type"] != current_type:
                # смена типа на том же уровне
                level = list_stack.pop()
                out.append(r"\end{itemize}" if level["type"] == "ul" else r"\end{enumerate}")
                out.append(r"\begin{enumerate}")
                list_stack.append({"type": current_type, "indent": indent})
            out.append(r"\item " + md_inline_to_tex(text))
            i += 1
            continue

        # Unordered list
        um = ul_re.match(line)
        if um:
            indent = len(um.group("indent") or "")
            text = um.group("text").strip()
            while list_stack and indent < list_stack[-1]["indent"]:
                level = list_stack.pop()
                out.append(r"\end{itemize}" if level["type"] == "ul" else r"\end{enumerate}")
            current_type = "ul"
            if not list_stack or indent > list_stack[-1]["indent"]:
                out.append(r"\begin{itemize}")
                list_stack.append({"type": current_type, "indent": indent})
            elif list_stack[-1]["type"] != current_type:
                level = list_stack.pop()
                out.append(r"\end{itemize}" if level["type"] == "ul" else r"\end{enumerate}")
                out.append(r"\begin{itemize}")
                list_stack.append({"type": current_type, "indent": indent})
            out.append(r"\item " + md_inline_to_tex(text))
            i += 1
            continue

        # Blank line
        if not line.strip():
            # Решаем, нужно ли закрывать списки.
            j = next_nonempty_index(i)
            if j is not None:
                next_line = lines[j]
                um_next = ul_re.match(next_line)
                om_next = ol_re.match(next_line)
                if (um_next or om_next) and list_stack:
                    # есть открытый список и дальше снова элемент списка.
                    # Проверяем, станет ли он вложенным относительно текущего уровня.
                    if um_next:
                        next_indent = len(um_next.group("indent") or "")
                    else:
                        next_indent = len(om_next.group("indent") or "")
                    # если отступ больше текущего уровня -> это вложенный список,
                    # НЕ закрываем окружение, только добавляем пустую строку.
                    if next_indent > list_stack[-1]["indent"]:
                        out.append("")
                        i += 1
                        continue
            # во всех остальных случаях пустая строка завершает все списки
            close_all_lists()
            out.append("")
            i += 1
            continue

        # Normal paragraph line
        close_all_lists()
        out.append(md_inline_to_tex(line))
        i += 1

    # Close any open blocks
    if in_code:
        out.append(r"\end{lstlisting}")
    close_quote()
    close_all_lists()

    # Ensure trailing newline
    return "\n".join(out).rstrip() + "\n"


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Convert Markdown (.md) to LaTeX (.tex) body."
    )
    parser.add_argument("--input", required=True, help="Input .md file.")
    parser.add_argument("--output", required=True, help="Output .tex file.")
    args = parser.parse_args()

    input_path = Path(args.input)
    output_path = Path(args.output)

    md_text = input_path.read_text(encoding="utf-8")
    body_tex = md_to_tex(md_text)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(body_tex, encoding="utf-8")


if __name__ == "__main__":
    main()  # py -m src.tools.md2tex --input docs/project_plan.md --output documentation/project_plan.tex
