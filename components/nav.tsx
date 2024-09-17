"use client";

import React from "react";
import { usePathname } from "next/navigation";
import Image from "next/image";
import Link from "next/link";

export function PageNav() {
  const pathname = usePathname();
  const isHome = pathname === "/";
  const isWriting = pathname.startsWith("/p");
  const logoTranslate = isHome ? "md:translate-x-28" : "md:translate-x-0";

  return (
    <nav className="flex justify-between pb-2 md:pb-4">
      <div className={`flex-none transition-transform ${logoTranslate}`}>
        <Image
          src="/logo.png"
          width={48}
          height={35.5}
          alt="AK label on an espresso cup"
        />
      </div>

      <ul className="flex justify-end">
        <li>
          <NavLink href="/" isSelected={isHome}>
            Home
          </NavLink>
        </li>
        <li>
          <NavLink href="/p" isSelected={isWriting}>
            Writing
          </NavLink>
        </li>
      </ul>
    </nav>
  );
}

function NavLink({
  href,
  isSelected,
  children,
}: {
  href: string;
  isSelected: boolean;
  children: React.ReactNode;
}) {
  return (
    <Link
      href={href}
      className={`block text-sm font-semibold leading-6 py-2 px-4 text-stone-600 hover:text-stone-400 dark:text-stone-300 dark:hover:text-stone-100 underline transition rounded-md ${isSelected ? "bg-stone-50 dark:bg-stone-800" : ""}`}
    >
      {children}
    </Link>
  );
}
