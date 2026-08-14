/** Shared markdown component overrides for react-markdown. Used by the chat
 *  message bubble, the agent test drawer, and any other component that
 *  renders LLM output as markdown. Centralizing the styles ensures
 *  consistent typography across the app. */
export const mdComponents = {
  h1: (p: any) => <h1 className="text-base font-bold mt-2 mb-1" {...p} />,
  h2: (p: any) => <h2 className="text-base font-bold mt-2 mb-1" {...p} />,
  h3: (p: any) => <h3 className="text-sm font-bold mt-2 mb-1" {...p} />,
  h4: (p: any) => <h4 className="text-sm font-semibold mt-1 mb-1" {...p} />,
  p: (p: any) => <p className="mb-2 last:mb-0 leading-relaxed" {...p} />,
  ul: (p: any) => <ul className="list-disc pl-4 mb-2 space-y-0.5" {...p} />,
  ol: (p: any) => <ol className="list-decimal pl-4 mb-2 space-y-0.5" {...p} />,
  li: (p: any) => <li className="leading-relaxed" {...p} />,
  a: (p: any) => <a className="underline hover:opacity-80" target="_blank" rel="noopener noreferrer" {...p} />,
  strong: (p: any) => <strong className="font-bold" {...p} />,
  blockquote: (p: any) => <blockquote className="border-l-2 pl-2 opacity-80 my-1" {...p} />,
  hr: (p: any) => <hr className="my-2 border-border/40" {...p} />,
  table: (p: any) => <table className="border-collapse my-2 text-xs w-full" {...p} />,
  th: (p: any) => <th className="border px-2 py-1 font-semibold text-left" {...p} />,
  td: (p: any) => <td className="border px-2 py-1" {...p} />,
  pre: (p: any) => <pre className="overflow-x-auto rounded bg-background/50 p-2 my-1 text-xs font-mono" {...p} />,
  code: ({ className, children, ...p }: any) => {
    const isBlock = /language-/.test(className || "");
    return isBlock
      ? <code className={className} {...p}>{children}</code>
      : <code className="px-1 py-0.5 rounded bg-background/50 font-mono text-xs" {...p}>{children}</code>;
  },
};
