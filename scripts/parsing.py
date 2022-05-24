"""The parsing module to parse the dumpped files.
"""

import json
import logging
import re

import pandas as pd

import constant as c


class FileParser:
    """
    The file parser for the files generated by multiverse-simulation.
    """

    def __init__(self, cd):
        """Initialize the parameters.

        Args:
            cd: The configuration dictionary.
        """
        self.x_axis_begin = cd['X_AXIS_BEGIN']
        self.colored_confirmed_like_items = c.COLORED_CONFIRMED_LIKE_ITEMS
        self.one_second = c.ONE_SEC
        self.target = c.TARGET

    def parse_aw_file(self, fn, variation):
        """Parse the accumulated weight files.

        Args:
            fc: The figure count.

        Returns:

        Returns:
            v: The variation value.
            data: The target data to analyze.
            x_axis: The scaled/adjusted x axis.
        """
        logging.info(f'Parsing {fn}...')
        # Get the configuration setup of this simulation
        # Note currently we only consider the first node
        config_fn = re.sub('aw0', 'aw', fn)
        config_fn = config_fn.replace('.csv', '.config')

        # Opening JSON file
        with open(config_fn) as f:
            c = json.load(f)

        v = str(c[variation])

        data = pd.read_csv(fn)

        # Chop data before the begining time
        data = data[data['ns since start'] >=
                    self.x_axis_begin * float(c["DecelerationFactor"])]

        # Reset the index to only consider the confirmed msgs from X_AXIS_BEGIN
        data = data.reset_index()

        # ns is the time scale of the aw outputs
        x_axis = float(self.one_second)
        data[self.target] = data[self.target] / float(c["DecelerationFactor"])
        return v, data[self.target], x_axis

    def parse_throughput_file(self, fn, var):
        """Parse the throughput files.
        Args:
            fn: The input file name.
            var: The variated parameter.

        Returns:
            v: The variation value.
            tip_pool_size: The pool size list.
            processed_messages: The # of processed messages list.
            issued_messages: The # of issued messages list.
            x_axis: The scaled x axis.
        """
        logging.info(f'Parsing {fn}...')
        # Get the configuration setup of this simulation
        config_fn = re.sub('tp', 'aw', fn)
        config_fn = config_fn.replace('.csv', '.config')

        # Opening JSON file
        with open(config_fn) as f:
            c = json.load(f)

        v = str(c[var])

        data = pd.read_csv(fn)

        # Chop data before the begining time
        data = data[data['ns since start'] >=
                    self.x_axis_begin * float(c["DecelerationFactor"])]

        # Get the throughput details
        tip_pool_size = data['UndefinedColor (Tip Pool Size)']
        processed_messages = data['UndefinedColor (Processed)']
        issued_messages = data['# of Issued Messages']

        # Return the scaled x axis
        x_axis = (data['ns since start'] / float(self.one_second) /
                  float(c["DecelerationFactor"]))
        return v, (tip_pool_size, processed_messages, issued_messages, x_axis)

    def parse_all_throughput_file(self, fn, var):
        """Parse the all-tp files.
        Args:
            fn: The input file name.
            var: The variated parameter.

        Returns:
            v: The variation value.
            tip_pool_size: The pool size list.
            x_axis: The scaled x axis.
        """
        logging.info(f'Parsing {fn}...')
        # Get the configuration setup of this simulation
        config_fn = re.sub('all-tp', 'aw', fn)
        config_fn = config_fn.replace('.csv', '.config')

        # Opening JSON file
        with open(config_fn) as f:
            c = json.load(f)

        v = str(c[var])

        data = pd.read_csv(fn)

        # Chop data before the begining time
        data = data[data['ns since start'] >=
                    self.x_axis_begin * float(c["DecelerationFactor"])]

        # Get the throughput details
        tip_pool_sizes = data.loc[:, data.columns != 'ns since start']

        # Return the scaled x axis
        x_axis = (data['ns since start'] / float(self.one_second) /
                  float(c["DecelerationFactor"]))
        return v, (tip_pool_sizes, x_axis)

    def parse_confirmed_color_file(self, fn, var):
        """Parse the confirmed color files.

        Args:
            fn: The input file name.
            var: The variated parameter.

        Returns:
            v: The variation value.
            colored_node_counts: The colored node counts list.
            convergence_time: The convergence time.
            flips: The flips count.
            unconfirming_blue: The unconfirming count of blue branch.
            unconfirming_red: The unconfirming count of red branch.
            total_weight: Total weight of all nodes in the network.
            x_axis: The scaled x axis.
        """
        logging.info(f'Parsing {fn}...')
        # Get the configuration setup of this simulation
        config_fn = re.sub('cc', 'aw', fn)
        config_fn = config_fn.replace('.csv', '.config')

        # Opening JSON file
        with open(config_fn) as f:
            c = json.load(f)

        data = pd.read_csv(fn)

        # Chop data before the begining time
        data = data[data['ns since start'] >=
                    self.x_axis_begin * float(c["DecelerationFactor"])]

        # Get the throughput details
        colored_node_aw = data[self.colored_confirmed_like_items]
        flips = data['Flips (Winning color changed)'].iloc[-1]

        # Unconfirmed Blue,Unconfirmed Red
        unconfirming_blue = data['Unconfirmed Blue'].iloc[-1]
        unconfirming_red = data['Unconfirmed Red'].iloc[-1]

        adversary_liked_aw_blue = data['Blue (Adversary Like Accumulated Weight)']
        adversary_liked_aw_red = data['Red (Adversary Like Accumulated Weight)']
        adversary_confirmed_aw_blue = data['Blue (Confirmed Adversary Weight)']
        adversary_confirmed_aw_red = data['Red (Confirmed Adversary Weight)']

        convergence_time = data['ns since issuance'].iloc[-1]
        convergence_time /= self.one_second
        convergence_time /= float(c["DecelerationFactor"])

        colored_node_aw["Blue (Like Accumulated Weight)"] -= adversary_liked_aw_blue
        colored_node_aw["Red (Like Accumulated Weight)"] -= adversary_liked_aw_red
        colored_node_aw["Blue (Confirmed Accumulated Weight)"] -= adversary_confirmed_aw_blue
        colored_node_aw["Red (Confirmed Accumulated Weight)"] -= adversary_confirmed_aw_red

        v = str(c[var])

        honest_total_weight = (c["NodesTotalWeight"] -
                               adversary_liked_aw_blue.iloc[-1] - adversary_liked_aw_red.iloc[-1])

        # Return the scaled x axis
        x_axis = ((data['ns since start']) /
                  float(self.one_second * float(c["DecelerationFactor"])))

        return v, (colored_node_aw, convergence_time, flips, unconfirming_blue, unconfirming_red,
                   honest_total_weight, x_axis)

    def parse_node_file(self, fn, var):
        """Parse the node files.
        Args:
            fn: The input file name.
            var: The variated parameter.

        Returns:
            v: The variation value.
            confirmation_rate_depth: The confirmation rate depth.
        """
        logging.info(f'Parsing {fn}...')
        # Get the configuration setup of this simulation
        config_fn = re.sub('nd', 'aw', fn)
        config_fn = config_fn.replace('.csv', '.config')

        # Opening JSON file
        with open(config_fn) as f:
            c = json.load(f)

        v = str(c[var])

        # Get the weight threshold
        weight_threshold = float(c['WeightThreshold'].split('-')[0])

        data = pd.read_csv(fn)

        # Get the minimum weight percentage
        mw = float(data['Min Confirmed Accumulated Weight'].min()
                   ) / float(c['NodesTotalWeight'])

        confirmation_rate_depth = max(weight_threshold - mw, 0) * 100.0

        return v, confirmation_rate_depth
