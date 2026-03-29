# Maestro: The Dark Tower Naming Map

**Version:** 1.0.0
**Date:** 2026-03-29

> *"The man in black fled across the desert, and the gunslinger followed."*
> -- Stephen King, The Gunslinger

This document provides a complete mapping between every component of the Maestro OCI container manager and the mythology of Stephen King's Dark Tower series. Each name was chosen so that the metaphor reinforces what the component actually does, making the codebase memorable and navigable for developers familiar with the series.

---

## Naming Philosophy

The Dark Tower series is, at its core, about a quest to reach and protect a structure that holds all of reality together. An OCI container manager is, at its core, about orchestrating isolated realities (containers) that are held together by shared infrastructure (images, networks, storage). The parallels run deep:

- **The Dark Tower** itself is the nexus where all realities converge -- like the core engine that orchestrates all subsystems.
- **The Beams** hold the Tower up and connect distant points -- like the network layer connecting containers.
- **Ka** (fate/destiny) drives everything forward -- like the state machine governing container lifecycles.
- **Todash space** is the void between worlds -- like the abstraction layers between components.
- **The Doorways** are portals between worlds -- like the registry client connecting to remote registries.
- **Gan** is the creator -- like the process that brings containers into existence.

---

## Complete Component Mapping

### 1. Core Engine / Orchestrator

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Main engine (orchestrates all subsystems) | **`tower`** | **The Dark Tower** -- Gan's body, the nexus of all realities, the structure at the center of everything | The Dark Tower is the axis upon which all worlds and all Beams depend. Without it, reality collapses. The core engine is the central orchestrator that every other subsystem depends on; if it fails, nothing works. Just as Roland's entire quest leads to the Tower, every CLI command and API call flows through this engine. |

---

### 2. Image Manager

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Content-addressable store (blob storage) | **`maturin`** | **Maturin the Turtle** -- Guardian of the Beam, the ancient turtle who carries a universe on his back and vomited the universe into existence | Maturin carries an entire reality on his shell. The content-addressable store carries all the blobs (layers, configs, manifests) that constitute every image in the system. It is the foundational substrate upon which everything else is built -- immutable, patient, and load-bearing. |
| Image pull logic | **`drawing`** | **The Drawing of the Three** -- the second novel, where Roland pulls companions from other worlds through magical doorways on the beach | Roland literally *draws* (pulls) people from another world into his own through doorways. Image pull logic draws images from remote registries into local storage. The act of reaching through a portal to bring something from another reality into yours is a perfect metaphor for `pull`. |
| Image push logic | **`unfound`** | **The Unfound Door** -- the concept of doors that can send things outward, and the idea of "sending" across worlds | In the series, doorways work in both directions. While "Drawing" pulls inward, pushing an image to a registry sends a local artifact outward to a remote world. The Unfound Door represents the reverse transit -- projecting local content outward to be discovered by others. |
| Garbage collection | **`reap`** | **Charyou Tree / Reaptide** -- the harvest festival bonfire ritual in Mejis where the old is burned away to make room for the new season | "Charyou tree" literally means "death tree" in High Speech, and Reaptide is the harvest festival where stuffy-guys (and in ancient times, sacrifices) are burned. Garbage collection is the reaping of unreferenced blobs, dangling layers, and orphaned images -- clearing out the dead to make space for the living. "Come reap." |
| Multi-platform selection | **`keystone`** | **Keystone World** -- the one true world among infinite parallel versions, the world where time only flows in one direction | Among all the parallel worlds in the Dark Tower multiverse, the Keystone World is the one that matters most -- the "real" one. Multi-platform selection examines a manifest index containing images for many platforms (linux/amd64, linux/arm64, etc.) and selects the one that matches *this* host -- finding your Keystone World among all possible variants. |

---

### 3. Container Lifecycle Manager

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Container creation | **`gan`** | **Gan** -- the creator god of the Dark Tower universe who brought all of existence into being | Gan is the Prime Creator. Container creation is the act of bringing a new isolated process-world into existence from an image blueprint. Just as Gan spoke the world into being, `gan` takes a container spec and manifests it as a real entity on disk with its own filesystem, namespace, and identity. |
| Container start/stop/kill | **`roland`** | **Roland Deschain** -- the last gunslinger, who controls life and death with his sandalwood guns, whose will determines who lives and who dies | Roland is the ultimate arbiter of action in the series. He starts quests, stops threats, and kills when necessary -- all with deliberate, practiced precision. Container start/stop/kill is the direct exercise of power over a container's running state: bringing it to life, suspending it, or ending it. Roland's guns are the interface to that power. |
| Container exec | **`touch`** | **The Touch** -- the psychic ability (possessed by Jake Chambers and others) to reach into another mind and operate within it | The Touch allows its wielder to reach into another being's consciousness and perceive or act within it. Container exec reaches into a running container's namespace and executes a command inside it. In both cases, you are projecting agency into an already-existing isolated world without disrupting it. |
| Container logs | **`glass`** (Wizard's Glass) | **Maerlyn's Rainbow / Wizard's Glass** -- the enchanted seeing spheres (especially the pink Grapefruit) that show visions of events happening elsewhere | The Wizard's Glass shows you what is happening or has happened in another place. Container logs let you observe the output stream of a process running inside an isolated namespace. Both are instruments of observation: you peer into the glass (or the log stream) and see what the container has been doing. |
| State machine (Created, Running, Paused, Stopped, Deleted) | **`ka`** | **Ka** -- destiny, fate, the force that drives all things forward; "ka is a wheel" | "Ka is a wheel; its only purpose is to turn." Ka governs the inevitable progression of all things through their ordained states. The container state machine governs the inevitable progression of a container through Created -> Running -> Paused -> Stopped -> Deleted. Each transition is driven by ka -- the deterministic lifecycle logic that ensures every container follows its destined path. The wheel turns. |

---

### 4. Runtime Abstraction

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Runtime interface (supports runc, crun, youki, gVisor, Kata) | **`eld`** | **Arthur Eld** -- the mythic ancestor-king whose bloodline produced all gunslingers; his sword Excalibur was reforged into Roland's sandalwood guns | Arthur Eld is the common ancestor from whom all gunslingers descend. The runtime interface is the common abstraction from which all OCI runtimes (runc, crun, youki, gVisor, Kata) descend. Eld defined the code of the gunslinger that all must follow; the interface defines the contract that all runtimes must implement. Different gunslingers, same lineage. Different runtimes, same interface. |
| Runtime auto-discovery | **`pathfinder`** | **The Path of the Beam** -- the invisible lines of force that guide travelers toward the Tower; following the Beam always leads you where you need to go | Following the Path of the Beam means sensing an invisible force and letting it guide you to what you seek. Runtime auto-discovery scans the system (PATH, well-known locations, configuration) to find available OCI runtimes without the user specifying them. Both involve detecting something you cannot directly see and following the trail to its source. |
| conmon-rs integration (container monitor/supervisor) | **`cort`** | **Cortland "Cort" Andrus** -- the brutal but dedicated weapons master of Gilead who trained all gunslinger apprentices and watched over them until they earned their guns | Cort is the tireless supervisor who watches over young gunslingers, ensuring they survive their training. conmon-rs is the tireless supervisor that watches over container processes, collecting their stdout/stderr, forwarding signals, and recording exit codes. Neither Cort nor conmon-rs is the main actor -- they are the disciplined watchers who ensure their charges complete their tasks. When the container process exits, conmon-rs faithfully records the result, just as Cort faithfully records whether an apprentice passed or failed. |

---

### 5. Network Manager

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Network manager (overall) | **`beam`** | **The Beams** -- the six invisible lines of force that connect the twelve Guardians in pairs and hold the Dark Tower upright at their intersection | The Beams are the connective infrastructure of all reality. They link distant Guardians across the entire world and converge at the Tower. The network manager is the connective infrastructure that links containers to each other and to the outside world. Without the Beams, the Tower falls and all worlds collapse. Without networking, containers are isolated and useless. |
| CNI plugin integration | **`guardian`** | **The Guardians of the Beam** -- the twelve great animal entities (Shardik the Bear, Maturin the Turtle, etc.) that anchor and protect the endpoints of each Beam | Each Guardian anchors one end of a Beam, providing the foundation point that the Beam connects to. CNI plugins are the specific implementations (bridge, macvlan, firewall, etc.) that anchor and implement each network configuration. The Guardian abstraction lets multiple different entities (Bear, Turtle, Wolf, Eagle...) serve the same structural role -- just as CNI lets multiple different plugins serve the same network role. |
| Network namespace lifecycle | **`todash`** | **Todash** -- the dark void between worlds; traveling "todash" means passing through the space between realities | Todash space is the boundary layer between worlds. A network namespace is a boundary layer that creates an isolated network reality for a container. Creating a netns is like opening todash space: you carve out a void and then populate it with interfaces and routes, building a world inside the void. Destroying it collapses that reality back into nothing. |
| Embedded DNS resolver | **`callahan`** | **Pere Callahan (Father Callahan)** -- the priest who translates between the mundane and the supernatural, who serves as the intermediary helping Roland's ka-tet navigate the Calla | Callahan is the translator and guide who helps people find what they need in an unfamiliar world. He knows everyone in Calla Bryn Sturgis by name. The embedded DNS resolver translates container names into IP addresses, helping containers find each other by name rather than by raw address. Both serve as the essential name-resolution layer that makes navigation possible. |
| Port mapping | **`doorway`** | **The Doorways Between Worlds** -- the magical and mechanical portals that connect one world to another at specific points | Each Doorway connects a specific point in one world to a specific point in another (e.g., the door on the beach connects Roland's world to 1960s New York). Port mapping connects a specific port on the host to a specific port inside the container. Both are precisely-targeted portals: host:8080 -> container:80, just as "The Prisoner" door leads specifically to Eddie Dean's New York. |
| Rootless networking (pasta/slirp4netns) | **`mejis`** | **Mejis** -- the remote, resource-constrained Out-World barony where young Roland had to accomplish his mission with limited authority and improvised means | In Mejis, teenage Roland operates far from Gilead's power structure, without the full authority of a proven gunslinger. He must accomplish his mission through cleverness and indirection rather than brute force. Rootless networking operates without root privileges, using userspace tools (pasta, slirp4netns) to accomplish network connectivity through clever indirection rather than direct kernel manipulation. Both succeed under constraints that would stop a less resourceful approach. |
| Default bridge network | **`trestle`** | **The Send Bridge / River Crossing trestle** -- the bridge at River Crossing where Roland's ka-tet crossed the river Send on their journey, and the long trestle that Blaine the Mono traverses | The bridge/trestle is the default path that travelers use to cross from one side to the other. The default bridge network (typically `maestro0`) is the default path that containers use to communicate. When you do not specify a network, traffic crosses the default bridge -- just as when you follow the main road in Mid-World, you cross the bridge at River Crossing. |

---

### 6. Storage Manager

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Snapshotter abstraction | **`prim`** | **The Prim** -- the primordial magical chaos from which all of reality was shaped; the raw, undifferentiated substrate of creation | The Prim is the raw magical substance from which Gan shaped the worlds. The snapshotter abstraction is the raw storage substrate from which container filesystems are materialized. Different drivers (overlay, btrfs, zfs) shape the Prim differently, but the underlying abstraction provides the common primordial interface. Both are the formless foundation that gets shaped into something usable. |
| OverlayFS driver | **`allworld`** | **All-World** -- the totality of layered realities stacked upon each other, the complete set of worlds that exist in the Dark Tower multiverse | All-World is the sum of all layered realities existing simultaneously, each one stacked upon and building from the ones below. OverlayFS works by stacking filesystem layers: a read-only lower layer, then another, then another, with a writable upper layer on top. The metaphor is structural -- All-World is literally overlaid realities, and OverlayFS is literally overlaid filesystems. |
| Volume management | **`dogan`** | **Dogan** -- the Old Ones' control rooms and storage facilities built by North Central Positronics; permanent installations that persist even as the world moves on | The Dogans are persistent, purpose-built storage and control facilities that outlast the civilization that created them. Volumes are persistent, purpose-built storage that outlasts the containers that use them. Both are durable infrastructure that exists independently of the transient entities that access them. A container is deleted, but its Dogan (volume) endures. |
| Layer diff/apply | **`palaver`** | **Palaver** -- the ritual of speaking and exchanging knowledge between gunslingers; to sit in palaver is to share information, compare perspectives, and reach understanding | Palaver is the structured exchange where differences are surfaced and reconciled. Layer diff computes the differences between two filesystem snapshots; layer apply takes a diff and reconciles it into an existing filesystem. Both are about comparing two states, identifying what changed, and integrating those changes. "Sit in palaver" and the layers will tell you what has changed. |

---

### 7. Registry Client

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Registry client (overall) | **`shardik`** | **Shardik the Bear** -- Guardian of one of the Beams; a massive cyborg bear built by North Central Positronics who guards the portal at his end of the Beam | Shardik guards the boundary between worlds -- the portal at his Beam's endpoint. The registry client guards the boundary between local storage and remote registries, mediating all communication across that boundary. Shardik is also a technological construct (a North Central Positronics cyborg), fitting for a client that speaks HTTP, handles OAuth tokens, and implements the OCI Distribution Spec -- technology mediating access to another world. |
| Authentication | **`sigul`** | **Sigul** -- the word for "seal" or "sign" in High Speech; the symbol of authority that proves identity (e.g., the sign of the Eld carved into Roland's guns) | A sigul is proof of identity and authority in Mid-World. Roland's guns bear the sigul of Eld, proving his lineage. Registry authentication presents credentials (tokens, certificates, passwords) that prove the client's identity and authorize access. Both are about presenting your mark to prove you have the right to pass. |
| Mirror/proxy resolution | **`thinny`** | **Thinny** -- a weak point in the fabric of reality where one world bleeds into another; a shimmering membrane where you can see and reach through to another version of the same place | A thinny is a place where the barrier between parallel realities is thin, allowing passage to an alternate version of the same location. A registry mirror is an alternate location that provides the same content as the primary registry. Both represent the same thing accessible through a different point in space. When the primary path is blocked, you find a thinny -- a mirror -- and reach the same content through the alternate route. |
| Retry + circuit breaker | **`horn`** | **The Horn of Eld** -- Arthur Eld's horn, a symbol of perseverance and second chances; Roland's story is a cycle, and each time through he gets another chance to do it right | The entire Dark Tower series is revealed to be a cycle: Roland reaches the Tower, is sent back to the beginning, and tries again. The Horn of Eld represents the hope that the next cycle will succeed where the last one failed. Retry logic sends the same request again when it fails, hoping the next attempt succeeds. The circuit breaker is the mechanism that recognizes when the cycle is broken -- when trying again will not help -- and stops the wheel from turning pointlessly. Ka is a wheel, but even ka knows when to rest. |

---

### 8. Artifact Manager

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| OCI artifact management | **`rose`** | **The Rose** -- the Dark Tower's twin in the Keystone World; a single wild rose growing in a vacant lot in New York City that contains an entire universe within its petals | The Rose is not the Tower itself, but an artifact that represents and is connected to the Tower -- a different manifestation of the same fundamental thing in a different world. OCI artifacts are not container images themselves, but related objects (Helm charts, WASM modules, signatures, SBOMs, policies) stored alongside images in the same registry infrastructure. The Rose is the artifact that lives in the image registry (the vacant lot on 46th and 2nd) alongside the mundane world. |
| Referrers API (discovering signatures, SBOMs attached to images) | **`nineteen`** | **Nineteen (19)** -- the mystical number that appears as a recurring sign throughout the series, guiding the ka-tet to pay attention and discover hidden connections between seemingly unrelated things | The number 19 appears again and again as a signal that connects disparate events, people, and objects. When the ka-tet sees 19, they know to look deeper and find the hidden relationship. The Referrers API discovers hidden relationships between artifacts -- finding the signature that is attached to an image, the SBOM linked to a build, the attestation connected to a deployment. Both are about following signs to discover what is connected to what. |

---

### 9. Security Manager

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Seccomp profile generation | **`white`** | **The White** -- the force of good, order, and protection that opposes the Crimson King's chaos; the fundamental power that gunslingers serve | The White is the protective force that maintains order against the chaos of the Outer Dark. Seccomp profiles are the protective system call filters that maintain order by blocking dangerous syscalls that could break containment. Both define what is *permitted* in order to keep chaos (exploits, escapes) at bay. The White does not attack -- it constrains and protects. |
| AppArmor/SELinux | **`gunslinger`** | **The Gunslingers of Gilead** -- the elite order of peacekeepers who enforce the law through a strict code of conduct and lethal skill | The gunslingers enforce the law of the land through a combination of strict rules (the gunslinger's creed) and the ability to act decisively when those rules are violated. AppArmor and SELinux enforce mandatory access control policies through strict rules (profiles/policies) and decisive enforcement (deny, audit, kill). Both are lawkeepers operating on a codified set of rules about what is and is not permitted. |
| Capabilities management | **`sandalwood`** | **The Sandalwood Guns** -- Roland's revolvers, forged from the steel of Excalibur, each carrying specific capabilities and the authority of the Eld | The sandalwood guns are instruments of specific, granted capability. Roland can draw, aim, and fire -- specific powers granted by possessing the guns. Linux capabilities are specific, granted powers (CAP_NET_ADMIN, CAP_SYS_PTRACE, etc.) that a process may or may not possess. Capabilities management decides which "guns" a container gets to carry. Drop a capability, and the container is disarmed of that specific power. |
| Image signing (cosign) | **`eld_mark`** | **The Sign of the Eld** -- the rose sigil carved into Roland's guns and displayed on the doors of Gilead's great hall; the mark that proves an artifact's authenticity and lineage | The Sign of the Eld is the mark of authenticity. When you see it on a gun, you know the gun is genuine and belongs to the line of Eld. Image signing (via cosign) places a cryptographic mark on an image that proves its authenticity and provenance. Both are unforgeable marks that answer the question: "Is this artifact genuine, and who made it?" |
| Rootless setup (user namespaces) | **`calla`** | **Calla Bryn Sturgis** -- the borderlands farming village that governs itself without the authority of Gilead's gunslingers; a self-sufficient community operating under its own local rules | The Calla operates independently, without the direct authority of Gilead (root). The people organize their own defense, manage their own affairs, and run their own community -- all without the central power structure. Rootless setup uses user namespaces to create an environment where the container manager operates with full apparent authority within its own scope, while having no actual root privileges on the host. The Calla is rootless Gilead. |

---

### 10. State Management

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| File-based state store | **`waystation`** | **The Way Station** -- the small shelter in the Mohaine Desert where Roland rested and found Jake; a persistent, reliable waypoint where travelers and information converge | The Way Station is a simple, durable structure in the desert that holds what travelers need: shelter, water, and -- critically -- Jake (information). The file-based state store is a simple, durable structure on disk that holds what the system needs: container states, image metadata, and network configs. Both are unassuming but essential waypoints that persist even as the world around them changes. |
| flock-based locking | **`khef`** | **Khef** -- the shared life-force that binds a ka-tet together; the deep connection that prevents members from acting at cross purposes; "sharing khef" means being in harmony | When a ka-tet shares khef, their actions are synchronized and they do not work against each other. flock-based locking ensures that concurrent processes (multiple CLI invocations) do not corrupt shared state by acting simultaneously on the same resource. Both are coordination mechanisms that prevent members of a group from colliding. "We share khef" means "we hold the lock." |
| State migrations | **`starkblast`** | **Starkblast** -- the sudden, violent storm in Mid-World that forces everything to take shelter and transforms the landscape; a disruptive but survivable event that changes the terrain | A starkblast is a violent weather event that reshapes the landscape and forces everyone to adapt or die. State migrations are schema changes that reshape the data store and require all components to adapt to the new structure. Both are disruptive transformations of the underlying terrain that must be survived and adapted to. When a starkblast comes, you hunker down and emerge into a changed world. When a migration runs, the state store is transformed and all consumers must understand the new shape. |

---

### 11. Service (Socket Mode)

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| gRPC/REST API server | **`positronics`** | **North Central Positronics** -- the ancient technology corporation of the Old Ones that built persistent infrastructure (robots, monorails, Dogans) serving anyone who knows how to interface with them | North Central Positronics built persistent technological services that outlive their creators and serve anyone who speaks their protocol. The API server is a persistent process that serves any client (CLI, TUI, IDE plugin, remote tool) that speaks gRPC or REST. Both are long-running service infrastructure that provides a standardized interface to powerful capabilities. |
| Event streaming | **`kashume`** | **Ka-shume** -- the premonition of approaching ka; the feeling that something significant is about to happen or has just happened; the signal that the wheel of ka is turning | Ka-shume is the awareness of events as they unfold in the stream of destiny -- the feeling of things happening. Event streaming provides real-time awareness of container events (start, stop, die, create, pull) as they unfold. Both are about broadcasting the signal that "something has happened" to those who are listening for it. |
| Background GC worker | **`breaker`** | **The Breakers** -- the psychics imprisoned in Algul Siento who use their mental powers to slowly, continuously erode the Beams | The Breakers sit in Blue Heaven and continuously, silently work to erode structures. The background GC worker sits as a background process and continuously, silently works to erode unused resources (orphaned layers, expired containers, dangling images). Both do their work quietly and persistently in the background, gradually reclaiming what is no longer needed. (Note: the Breakers are villains, but the GC worker is heroic -- it reclaims waste rather than destroying infrastructure. The structural metaphor of "background process continuously eroding things" is what fits.) |

---

### 12. TUI Dashboard

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Main dashboard app | **`glass`** | **Maerlyn's Rainbow (the Wizard's Glasses)** -- the thirteen enchanted seeing spheres that show visions of events across all worlds | Maerlyn's Rainbow lets the viewer see what is happening across multiple worlds simultaneously. The TUI dashboard shows the state of containers, images, networks, and volumes -- multiple "worlds" visible at a glance. The dashboard is the developer's scrying glass: look into it and see everything that is happening in your Maestro-managed universe. |
| Container list view | **`midworld`** | **Mid-World** -- the vast, mapped landscape where containers (people, places, things) exist and can be surveyed | Mid-World is the primary landscape where everything happens -- the world you survey and navigate. The container list view is the primary landscape of running/stopped containers that you survey and navigate. When you look at Mid-World, you see the lay of the land. When you look at the container list, you see the lay of your containers. |
| Image list view | **`endworld`** | **End-World** -- the remote, vast territory beyond Mid-World containing the Dark Tower, Thunderclap, and the accumulated artifacts of ages | End-World contains the accumulated structures and artifacts of the entire Dark Tower mythology. The image list contains the accumulated artifacts (pulled images, built images, cached layers) of your container work. Both are repositories of everything that has been gathered and stored. |
| Log viewer | **`oracle`** | **The Oracle / Maerlyn's Grapefruit (Pink Glass)** -- the seeing stone that shows detailed visions of specific events, addictively compelling to watch | The Wizard's Glass (especially the pink Grapefruit) shows detailed, flowing visions of specific events -- and is dangerously addictive to watch. The log viewer shows the detailed, flowing output of a specific container -- and can be equally hard to stop watching (`tail -f`, anyone?). Both are instruments for peering deep into the details of a single subject. |

---

### 13. CLI Layer

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Root command | **`dinh`** | **Dinh** -- the leader of a ka-tet; the one who commands and from whom all orders flow; Roland is dinh of his ka-tet | The dinh is the root of command authority. All instructions originate from the dinh, and all members of the ka-tet look to the dinh for direction. The root command is the entry point from which all subcommands branch. `maestro` (the root command) is the dinh, and all subcommands (`container`, `image`, `volume`, etc.) are members of the ka-tet who follow the dinh's lead. |
| Container subcommands | **`gunslinger`** | **Gunslinger commands** -- Roland's direct actions in the field: draw, fire, reload, holster | In the series, gunslinger actions are immediate, physical, and deal with living/dying things. Container subcommands (run, stop, kill, exec, logs, inspect) are the immediate, operational commands that deal with the living/dying entities in the system. These are the "draw and fire" commands of Maestro. |
| Image subcommands | **`archivist`** | **The archivists and librarians** -- characters like Aaron Deepneau and Calvin Tower who collect, catalog, and protect important artifacts (especially the vacant lot and the Rose) | Calvin Tower and Aaron Deepneau are the custodians of the Rose's vacant lot -- they catalog, protect, and manage access to critical artifacts. Image subcommands (pull, push, list, inspect, rm, tag) are the custodial operations for managing the image catalog. Both are about careful stewardship of stored artifacts. |
| Volume subcommands | **`keeper`** | **The keepers** -- those who maintain and guard persistent places and structures throughout Mid-World (e.g., the keeper of the Way Station, the keepers of the Dogan) | Keepers are the stewards of persistent infrastructure. Volume subcommands (create, inspect, rm, ls, prune) are the stewardship operations for persistent storage. Both are about maintaining and governing durable things that outlast transient visitors. |
| Network subcommands | **`beamseeker`** | **Beam-seekers** -- those who follow and maintain the Beams; Roland's ka-tet are beam-seekers for much of their journey | The ka-tet spends much of their journey following, protecting, and troubleshooting the Beams. Network subcommands (create, inspect, rm, ls, connect, disconnect) are the operations for following, configuring, and troubleshooting network connectivity. Both are about the care and feeding of the connective infrastructure. |
| Artifact subcommands | **`collector`** | **Collectors of Maerlyn's Rainbow** -- those who seek out, manage, and trade the enchanted artifacts (the colored glasses) that exist alongside the mundane world | Throughout the series, various characters seek, possess, and trade the glasses of Maerlyn's Rainbow -- artifacts of power that exist alongside mundane objects. Artifact subcommands (push, pull, discover, attach, list) manage OCI artifacts that exist alongside container images in registries. Both deal in the special objects that coexist with the ordinary ones. |
| System subcommands | **`antet`** | **An-tet** -- to speak an-tet means to be completely open and transparent, sharing all information; to sit in council and examine the full state of affairs | An-tet is the ritual of full transparency and assessment -- laying everything on the table. System subcommands (info, version, prune, df, events) are the operations for examining the full state of the system, reporting disk usage, and performing system-wide maintenance. Both are about taking stock of everything: honest, complete system introspection. |

---

### 14. Configuration and Infrastructure

| Component | Dark Tower Name | Reference | Justification |
|-----------|----------------|-----------|---------------|
| Config file | **`katet.toml`** | **Ka-tet** -- a group of people bound by destiny who are stronger together than apart; the configuration that defines how the group works as a unit | A ka-tet is the binding that defines the relationships, roles, and shared purpose of its members. The configuration file defines the relationships, settings, and shared parameters of all Maestro components. Both are the declaration of "how we work together." When you edit `katet.toml`, you are reshaping the bonds of the ka-tet. |
| Default network name | **`beam0`** | **The first Beam (Bear-Turtle, Shardik-Maturin)** -- the primary Beam that Roland's ka-tet follows for most of their journey; the default path | The Shardik-Maturin Beam is the first and primary Beam the ka-tet encounters and follows. It is the default path. `beam0` is the default network that containers are connected to when no other network is specified. Both are the "if you don't choose otherwise, follow this one" default path. |
| Lock directory | **`thinnies`** | **Thinny locations** -- specific, marked danger points where the fabric of reality is thin and concurrent realities risk bleeding into each other | Thinnies are places where concurrent realities risk colliding and corrupting each other. Lock files are placed where concurrent processes risk colliding and corrupting shared state. Both mark the specific locations where "here be dangerous concurrency" and where careful coordination is required. The lock directory is the map of all known thinnies. |

---

## Summary Quick Reference

| Maestro Component | Dark Tower Name | One-line mnemonic |
|---|---|---|
| Core Engine | `tower` | The nexus of all realities |
| Content Store | `maturin` | The turtle carries all worlds |
| Image Pull | `drawing` | Drawing of the Three (pulling from other worlds) |
| Image Push | `unfound` | The Unfound Door (sending outward) |
| Garbage Collection | `reap` | Charyou tree -- come reap the dead |
| Multi-platform Select | `keystone` | Finding *your* world among all worlds |
| Container Create | `gan` | The creator god |
| Container Start/Stop/Kill | `roland` | The gunslinger's will over life and death |
| Container Exec | `touch` | The Touch -- reaching into another mind |
| Container Logs | `glass` | Wizard's Glass -- seeing what happens elsewhere |
| Container State Machine | `ka` | Ka is a wheel; its only purpose is to turn |
| Runtime Interface | `eld` | Arthur Eld -- ancestor of all gunslingers |
| Runtime Discovery | `pathfinder` | Following the Path of the Beam |
| conmon-rs Integration | `cort` | The weapons master who watches over trainees |
| Network Manager | `beam` | The Beams hold everything together |
| CNI Plugin Integration | `guardian` | Guardians anchor the Beams |
| Network Namespace | `todash` | The void between worlds |
| DNS Resolver | `callahan` | The priest who knows everyone's name |
| Port Mapping | `doorway` | Portals connecting specific points between worlds |
| Rootless Networking | `mejis` | Succeeding without Gilead's authority |
| Default Bridge | `trestle` | The bridge at River Crossing |
| Snapshotter | `prim` | The primordial substrate of creation |
| OverlayFS Driver | `allworld` | Layered realities stacked on each other |
| Volume Management | `dogan` | Persistent storage facilities of the Old Ones |
| Layer Diff/Apply | `palaver` | Exchanging and reconciling knowledge |
| Registry Client | `shardik` | The Guardian at the boundary |
| Authentication | `sigul` | The seal that proves identity |
| Mirror/Proxy | `thinny` | A weak point where you can reach an alternate path |
| Retry/Circuit Breaker | `horn` | The horn of second chances; ka's cycle |
| Artifact Manager | `rose` | The Tower's twin -- artifact alongside the real thing |
| Referrers API | `nineteen` | The number that reveals hidden connections |
| Seccomp Profiles | `white` | The force of order and protection |
| AppArmor/SELinux | `gunslinger` | Elite enforcers of the law |
| Capabilities Mgmt | `sandalwood` | The guns -- specific, granted powers |
| Image Signing | `eld_mark` | The Sign of Eld -- proof of authenticity |
| Rootless Setup | `calla` | Self-governing without central authority |
| State Store | `waystation` | The durable waypoint in the desert |
| flock Locking | `khef` | Shared life-force that synchronizes the ka-tet |
| State Migrations | `starkblast` | The storm that reshapes the landscape |
| API Server | `positronics` | North Central Positronics -- persistent services |
| Event Streaming | `kashume` | The premonition that ka is turning |
| Background GC | `breaker` | Quietly eroding what is no longer needed |
| TUI Dashboard | `glass` | Maerlyn's Rainbow -- seeing all worlds |
| Container List | `midworld` | The landscape you survey |
| Image List | `endworld` | The repository of accumulated artifacts |
| Log Viewer | `oracle` | The seeing stone for detailed visions |
| Root Command | `dinh` | The leader from whom all commands flow |
| Container Cmds | `gunslinger` | Draw, fire, reload -- immediate action |
| Image Cmds | `archivist` | Custodians of important artifacts |
| Volume Cmds | `keeper` | Stewards of persistent infrastructure |
| Network Cmds | `beamseeker` | Following and maintaining the Beams |
| Artifact Cmds | `collector` | Seekers of Maerlyn's Rainbow |
| System Cmds | `antet` | Full transparency -- laying it all on the table |
| Config File | `katet.toml` | The binding that defines the group |
| Default Network | `beam0` | The first and default Beam |
| Lock Directory | `thinnies` | Where concurrent realities risk collision |

---

## Appendix: Dark Tower Source Material

The naming in this document draws from the following works by Stephen King:

1. **The Gunslinger** (1982/2004) -- Roland, the Man in Black, the Way Station, the Mohaine Desert
2. **The Drawing of the Three** (1987) -- the Doorways, Eddie Dean, Susannah Dean, Drawing/pulling from other worlds
3. **The Waste Lands** (1991) -- Shardik, Blaine the Mono, Lud, the Path of the Beam, Jake's return
4. **Wizard and Glass** (1997) -- Mejis, Maerlyn's Rainbow, Rhea of the Coos, Charyou Tree, the Big Coffin Hunters
5. **Wolves of the Calla** (2003) -- Calla Bryn Sturgis, Pere Callahan, Andy the Robot, the Manni, Black Thirteen
6. **Song of Susannah** (2004) -- the Keystone World, Mia, the Dogan, North Central Positronics, Todash
7. **The Dark Tower** (2004) -- the Crimson King, the Breakers, Algul Siento, Can'-Ka No Rey, Patrick Danville, the field of roses, the Tower rooms, the Horn of Eld, "ka is a wheel"
8. **The Wind Through the Keyhole** (2012) -- the Starkblast, the Skin-Man, Tim Ross, the Covenant Man, Maerlyn

### Glossary of High Speech Terms Used

| Term | Pronunciation | Meaning |
|------|--------------|---------|
| Ka | kah | Destiny, fate, the force that drives all things |
| Ka-tet | kah-TET | A group bound by destiny |
| Dinh | din | Leader, father of the group |
| Khef | kef | Life-force, spirit, the water of life |
| An-tet | an-TET | Full sharing, complete transparency |
| Ka-shume | kah-SHOOM | Premonition of approaching destiny |
| Sigul | SIG-ul | Seal, sign, mark of authority |
| Todash | TOE-dash | The void between worlds |
| Gan | gan | The creator god |
| Prim | prim | The primordial magical chaos |
| Eld | eld | Ancient, of the old line |
| Palaver | pah-LAV-er | Council, structured conversation |

---

> *"Go then, there are other worlds than these."* -- Jake Chambers
