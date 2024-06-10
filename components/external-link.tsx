import React from "react";

export function ExternalLink({
  href,
  children,
}: {
  href: string;
  children: React.ReactNode;
}) {
  return (
    <a
      href={href}
      className="text-stone-600 hover:text-stone-400 dark:text-stone-300 dark:hover:text-stone-100 underline transition"
    >
      {children}
    </a>
  );
}
