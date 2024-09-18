import { Disclosure } from "@/components/disclosure";
import React from "react";
import { ExternalLink } from "@/components/external-link";

export default function HomePage() {
  return (
    <>
      <Section id="name">
        <h1>
          <a href="/" className="flex">
            <SectionHeader>Alex</SectionHeader>
            <SectionBody className="text-red-600">Kern</SectionBody>
          </a>
        </h1>
      </Section>

      <Section id="currently">
        <SectionHeader>Currently</SectionHeader>
        <SectionBody>
          <List>
            <ListItem>
              <ExternalLink href="https://figma.com/">Figma</ExternalLink>, New
              Products
            </ListItem>
          </List>
        </SectionBody>
      </Section>

      <Section id="previously">
        <SectionHeader>Previously</SectionHeader>
        <SectionBody>
          <List>
            <ListItem>
              <ExternalLink href="https://dynaboard.com/">
                Dynaboard
              </ExternalLink>
              , Founder &amp; CEO
              <SubBlock>
                acq. <ExternalLink href="https://figma.com">Figma</ExternalLink>
              </SubBlock>
            </ListItem>
            <ListItem>
              <ExternalLink href="https://distributedsystems.com/">
                Distributed Systems
              </ExternalLink>
              , Cofounder &amp; CTO
              <SubBlock>
                acq.{" "}
                <ExternalLink href="https://coinbase.com">
                  Coinbase
                </ExternalLink>
              </SubBlock>
            </ListItem>
            <ListItem>
              <ExternalLink href="https://coinbase.com/">Coinbase</ExternalLink>
              , Crypto Tech Lead
            </ListItem>
            <ListItem>
              <ExternalLink href="https://calhacks.io">Cal Hacks</ExternalLink>,
              Founder
            </ListItem>
            <ListItem>
              <ExternalLink href="https://www.imdb.com/name/nm11088678/">
                HBO Silicon Valley
              </ExternalLink>
              , Technical Advisor
            </ListItem>
            <ListItem>
              Forbes 30 <Sub>under</Sub> 30
            </ListItem>
            <Disclosure>
              <ListItem>Zebra IQ, CTO &amp; Cofounder</ListItem>
              <ListItem>
                <ExternalLink href="https://www.alsop-louie.com/">
                  Alsop-Louie Partners
                </ExternalLink>
                , Associate
              </ListItem>
              <ListItem>
                <ExternalLink href="https://apple.com">Apple</ExternalLink>,
                Applied Machine Learning
              </ListItem>
              <ListItem>
                <ExternalLink href="https://mars.jpl.nasa.gov/msl/">
                  NASA JPL
                </ExternalLink>
                , Software Engineering
              </ListItem>
              <ListItem>
                <ExternalLink href="https://www.berkeley.edu/">
                  UC Berkeley
                </ExternalLink>
                , Computer Science
              </ListItem>
              <ListItem>Kairos Society, Regional President</ListItem>
              <ListItem>
                FRC Team 1515{" "}
                <ExternalLink href="https://www.team1515.com/">
                  Mortorq
                </ExternalLink>
                , Team Captain
              </ListItem>
              <ListItem>
                <ExternalLink href="https://www.planetbravo.com/">
                  PlanetBravo Techno-Tainment Camp
                </ExternalLink>
                , CIT
              </ListItem>
            </Disclosure>
          </List>
        </SectionBody>
      </Section>

      <Section id="links">
        <SectionHeader>Links</SectionHeader>
        <SectionBody>
          <ExternalLink href="https://x.com/kernio">X / Twitter</ExternalLink>,{" "}
          <ExternalLink href="https://github.com/kern">GitHub</ExternalLink>,{" "}
          <ExternalLink href="https://www.linkedin.com/in/alexanderskern/">
            LinkedIn
          </ExternalLink>
          , <ExternalLink href="mailto:hello@kern.io">Email</ExternalLink>
        </SectionBody>
      </Section>

      <Section id="projects">
        <SectionHeader>Projects</SectionHeader>
        <SectionBody>
          <ExternalLink href="https://file.pizza">FilePizza</ExternalLink>,{" "}
          <ExternalLink href="https://github.com/kern/ditto">
            Ditto
          </ExternalLink>
        </SectionBody>
      </Section>
    </>
  );
}

function Section({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <section id={id} className="flex mb-2 text-lg flex-col md:flex-row">
      {children}
    </section>
  );
}

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="md:w-32 md:text-right text-stone-400 dark:text-stone-200 font-light">
      {children}
    </h2>
  );
}

function SectionBody({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={`grow pl-2 md:pl-4 font-semibold ${className}`}>
      {children}
    </div>
  );
}

function List({ children }: { children: React.ReactNode }) {
  return <ul>{children}</ul>;
}

function ListItem({ children }: { children: React.ReactNode }) {
  return <li className="mb-2">{children}</li>;
}

function Sub({ children }: { children: React.ReactNode }) {
  return (
    <span className="text-xs text-stone-700 dark:text-stone-200">
      {children}
    </span>
  );
}

function SubBlock({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-xs text-stone-700 dark:text-stone-200">{children}</div>
  );
}
