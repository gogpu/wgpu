# Particle simulation

This is a very simple particle simulation system. It does not have much resemblance to actual physics.
The features implemented:

1. The particles repell each other at close distances. No attraction forces between particles.
2. They bounce off the walls and off the central circle.
3. They are colored according to their mass (rainbow colors with red being the heaviest)
4. They are depiceted as simple squares with the edge size of `³√mass`
5. They gravitate towards the central circle

The goal of the simulation is two-fold:
  * produce something visually similar to a planet's atomosphere
  * demonstrate a multi-step compute pipeline

Because of different masses the heavier particles tend to settle towards the central circle so eventually (10-20 minutes at 144fps)
you should see something like this:
![Eventual equilibrium](screenshot.png)

# Further development
This project (and possibly some others) is planned to be further developed at

[github.com/snakeru/wgpu-particles](https://github.com/snakeru/wgpu-particles)
