"use client";

import React, { useState } from "react";

export function Disclosure({ children }: { children: React.ReactNode }) {
  const [isDisclosureVisible, setDisclosureVisible] = useState(false);

  const handleMoreClick = () => {
    setDisclosureVisible(true);
  };

  if (!isDisclosureVisible) {
    return (
      <div
        aria-label="See more"
        className="cursor-pointer text-xs text-stone-400"
        onClick={handleMoreClick}
      >
        See more &raquo;
      </div>
    );
  }

  return <>{children}</>;
}
