// Portraits for the agent names in internal/core/consts.go (AgentNames): small drawings in each
// character's colours, made here rather than copied from the show. Renamed agents keep initials.
const P = {
  jerry:     { skin: "#f2cfb3", hair: "#c9ccd1", hairStyle: "receding", outfit: "#3b4252", tie: "#b3261e", bg: "#dfe3ea" },
  sam:       { skin: "#f6d3b8", hair: "#c2410c", hairStyle: "long", outfit: "#16a34a", bg: "#dcfce7" },
  clover:    { skin: "#f8d7bf", hair: "#facc15", hairStyle: "bob", outfit: "#dc2626", bg: "#fee2e2" },
  alex:      { skin: "#8d5a3b", hair: "#1c1917", hairStyle: "short", outfit: "#eab308", bg: "#fef9c3" },
  britney:   { skin: "#9a6646", hair: "#1c1917", hairStyle: "long", outfit: "#2563eb", bg: "#dbeafe" },
  blaine:    { skin: "#f2cfb3", hair: "#eab308", hairStyle: "short", outfit: "#1d4ed8", bg: "#dbeafe" },
  mandy:     { skin: "#f3d2b6", hair: "#27272a", hairStyle: "long", outfit: "#db2777", bg: "#fce7f3" },
  arnold:    { skin: "#f2cfb3", hair: "#78350f", hairStyle: "short", outfit: "#4d7c0f", bg: "#ecfccb", glasses: true },
  dean:      { skin: "#f2cfb3", hair: "#57534e", hairStyle: "short", outfit: "#0f766e", bg: "#ccfbf1" },
  mike:      { skin: "#e8b894", hair: "#92400e", hairStyle: "short", outfit: "#ea580c", bg: "#ffedd5" },
  gladis:    { robot: true, outfit: "#0e7490", bg: "#cffafe" },
  caitlin:   { skin: "#f6d3b8", hair: "#7c2d12", hairStyle: "long", outfit: "#c026d3", bg: "#fae8ff" },
  dominique: { skin: "#f8d7bf", hair: "#fde047", hairStyle: "long", outfit: "#e11d48", bg: "#ffe4e6" },
  tim:       { skin: "#f2cfb3", hair: "#44403c", hairStyle: "short", outfit: "#18181b", tie: "#7f1d1d", bg: "#e7e5e4" },
  terrence:  { skin: "#f2cfb3", hair: "#a1a1aa", hairStyle: "receding", outfit: "#6b21a8", tie: "#facc15", bg: "#f3e8ff" },
  helga:     { skin: "#f5d5bb", hair: "#fde68a", hairStyle: "bun", outfit: "#262626", bg: "#e5e5e5" },
  geraldine: { skin: "#f0cdb0", hair: "#d4d4d8", hairStyle: "bun", outfit: "#7f1d1d", bg: "#fee2e2", glasses: true },
  gus:       { skin: "#c68863", hair: "#3f3f46", hairStyle: "short", outfit: "#f97316", bg: "#ffedd5" },
};

// "Sam" and "Sam 2" (names wrap with a number once all are taken) get the same portrait.
export const portrait = name => P[String(name || "").trim().split(/\s+/)[0].toLowerCase()] || null;
