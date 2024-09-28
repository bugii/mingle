import { Command } from 'commander';
import { exec } from 'child_process';
import * as yaml from 'yaml';
import * as path from 'path';
import * as fs from 'fs';
import os from 'os';

type Session = {
  name: string;
  path?: string;
  type?: 'worktreeroot';
  tmuxinator?: string;
};

type ConfigSession =
  | {
      type: undefined;
      path: string;
      tmuxinator?: string;
    }
  | {
      type: 'worktreeroot';
      path: string;
      tmuxinator?: string;
    };

const CONFIG_PATH = path.join(os.homedir(), '.config/workflow/workflow.yaml');

async function loadConfig(): Promise<ConfigSession[]> {
  return new Promise((resolve, reject) => {
    try {
      if (!fs.existsSync(CONFIG_PATH)) {
        console.info('No config file was found');
        return resolve([]);
      }

      const configFile = fs.readFileSync(CONFIG_PATH, 'utf8');
      const config = yaml.parse(configFile) as ConfigSession[];

      resolve(config);
    } catch (error) {
      reject(error);
    }
  });
}

const program = new Command();

program.name('workflow').description('tool to improve my workflow');

program
  .command('list')
  .description('list all available sessions')
  .action(async () => {
    try {
      const sessions = await getSessions();
      writeToStdout(sessions.map((s) => s.name));
    } catch (error) {
      console.error(error);
    }
  });

program
  .command('connect')
  .description('connect to a given session')
  .argument('<session>', 'session to create/connect to')
  .action(async (session) => {
    try {
      //TODO: def. could be improved :)
      const sessions = await getSessions();
      const tmuxSessions = await getTmuxSessions();
      // const selected = await writeToStdout(sessions.map((s) => s.name));
      // we have nothing better than the name at this point, as this is the only identifier for tmux sessions :)
      const selectedSession = sessions.find((s) => s.name === session)!;
      if (tmuxSessions.find((s) => s.name === selectedSession.name) === undefined) {
        await createTmuxSession(selectedSession);
      }
      await switchToTmuxSession(selectedSession.name);
    } catch (error) {
      console.error(error);
    }
  });

program.parse();

async function getSessions() {
  const config: ConfigSession[] = await loadConfig();

  const tmuxSessions: Session[] = await getTmuxSessions();
  const zoxideSessions: Session[] = await getZoxideResults();
  const configWorktreeSessions: Session[] = [];
  const configSessions: Session[] = [];

  for (const c of config) {
    if (c.type === 'worktreeroot') {
      const worktrees = await getGitWorktrees(c.path);
      configWorktreeSessions.push(...worktrees.map((w) => ({ ...c, path: w, name: w })));
    } else {
      configSessions.push({ ...c, name: c.path });
    }
  }

  // determine order and stuff
  // dots are not allowed in tmux session names
  const sessions = [...tmuxSessions, ...configSessions, ...configWorktreeSessions, ...zoxideSessions].map((s) => ({
    ...s,
    name: s.name.replace(/\./g, '_'),
  }));

  return sessions;
}

async function getTmuxSessions(): Promise<Session[]> {
  return new Promise((resolve, reject) => {
    exec('tmux list-sessions -F "#{session_name}"', (error, stdout, stderr) => {
      if (error) {
        return reject(`Error executing tmux command: ${stderr}`);
      }
      const sessions = stdout
        .split('\n')
        .filter((session) => session.trim() !== '')
        .map((s) => ({ name: s }));
      resolve(sessions);
    });
  });
}

function getZoxideResults(): Promise<Session[]> {
  return new Promise((resolve, reject) => {
    exec('zoxide query -l', (error, stdout, stderr) => {
      if (error) {
        return reject(`Error executing zoxide command: ${stderr}`);
      }
      const results = stdout
        .split('\n')
        .filter((path) => path.trim() !== '')
        .map((p) => ({ name: p, path: p }));
      resolve(results);
    });
  });
}

function writeToStdout(data: string[]): void {
  console.log(data.join('\n'));
}

function switchToTmuxSession(sessionName: string): Promise<void> {
  return new Promise((resolve, reject) => {
    exec(`tmux switch-client -t ${sessionName}`, (error, _stdout, stderr) => {
      if (error) {
        console.error(`Error switching to tmux session: ${stderr}`);
        return reject(new Error(stderr));
      }
      // console.log(`Switched to tmux session: ${sessionName}`);
      resolve();
    });
  });
}

function createTmuxSession(session: Session): Promise<void> {
  return new Promise((resolve, reject) => {
    if (!session.path) {
      throw new Error('Session path is missing, cannot create session');
    }
    if (session.tmuxinator) {
      exec(
        `cd ${session.path} && yes | tmuxinator start -n ${session.name} -p ${session.tmuxinator} --no-attach`,
        (error, _stdout, stderr) => {
          if (error) {
            console.error(`Error starting tmuxinator session: ${stderr}`);
            return reject(new Error(stderr));
          }
          // console.log(`Created new tmux session: ${session.name}`);
          return resolve();
        }
      );
    } else {
      exec(`tmux new-session -s ${session.name} -d -c ${session.path}`, (error, _stdout, stderr) => {
        if (error) {
          console.error(`Error creating new tmux session: ${stderr}`);
          return reject(new Error(stderr));
        }
        // console.log(`Created new tmux session: ${session.name}`);
        resolve();
      });
    }
  });
}

async function getGitWorktrees(worktreeRoot: string): Promise<string[]> {
  return new Promise((resolve, reject) => {
    exec(`git -C ${worktreeRoot} worktree list --porcelain`, (error, stdout, stderr) => {
      if (error) {
        return reject(`Error fetching git worktrees: ${stderr}`);
      }
      const worktrees = stdout
        .split('\n')
        .filter((line) => line.startsWith('worktree '))
        .map((line) => line.replace('worktree ', '').trim());
      resolve(worktrees);
    });
  });
}
